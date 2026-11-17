// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// GPU node roles read from DCGM and kube-state-metrics series, so a GPU suite finds
// its idle, busy and multi-GPU nodes on any cluster instead of by instance type.

// GPUResource is the kube-state-metrics resource label value for nvidia.com/gpu.
const GPUResource = "nvidia_com_gpu"

// GPUNode is one node and its DCGM series.
type GPUNode struct {
	Name   string
	Series []MetricResult
}

// GPUs returns the sorted distinct non-empty values of the gpu datapoint attribute.
func (n GPUNode) GPUs() []string {
	seen := make(map[string]struct{})
	for _, r := range n.Series {
		if v := r.Labels.Datapoint["gpu"]; v != "" {
			seen[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Idle reports whether no series of the node carries k8s.pod.name, so no pod holds a GPU.
func (n GPUNode) Idle() bool {
	for _, r := range n.Series {
		if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
			return false
		}
	}
	return true
}

// Runs reports whether a pod whose name starts with podPrefix holds one of the node's GPUs.
func (n GPUNode) Runs(podPrefix string) bool {
	for _, r := range n.Series {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], podPrefix) {
			return true
		}
	}
	return false
}

// GPURoles sorts the nodes of DCGM series by role. A node can have more than one role.
type GPURoles struct {
	Nodes []GPUNode // every node, sorted by name
	Idle  []GPUNode // no pod holds a GPU
	Busy  []GPUNode // a pod with the busy prefix holds a GPU
}

// ReadGPURoles groups DCGM series by resource k8s.node.name and sorts the nodes by role.
// Series without k8s.node.name are left out. The GPU suite's attribute schema requires it.
func ReadGPURoles(results []MetricResult, busyPodPrefix string) GPURoles {
	byNode := make(map[string][]MetricResult)
	for _, r := range results {
		if node := r.Labels.Resource["k8s.node.name"]; node != "" {
			byNode[node] = append(byNode[node], r)
		}
	}
	var roles GPURoles
	for name, series := range byNode {
		roles.Nodes = append(roles.Nodes, GPUNode{Name: name, Series: series})
	}
	sort.Slice(roles.Nodes, func(i, j int) bool { return roles.Nodes[i].Name < roles.Nodes[j].Name })
	for _, n := range roles.Nodes {
		if n.Idle() {
			roles.Idle = append(roles.Idle, n)
		}
		if n.Runs(busyPodPrefix) {
			roles.Busy = append(roles.Busy, n)
		}
	}
	return roles
}

// MultiGPUNodes returns the nodes whose allocatable, from GPUAllocatable, is more than
// one GPU. The allocatable decides, so a node whose DCGM series report too few gpu
// values is still a multi-GPU node. It returns an error for a node with no allocatable.
func MultiGPUNodes(nodes []GPUNode, allocatable map[string]int) ([]GPUNode, error) {
	var out []GPUNode
	var missing []string
	for _, n := range nodes {
		gpus, ok := allocatable[n.Name]
		if !ok {
			missing = append(missing, n.Name)
			continue
		}
		if gpus > 1 {
			out = append(out, n)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no %s allocatable for nodes %v", GPUResource, missing)
	}
	return out, nil
}

// SeriesOnNodes returns the results whose resource k8s.node.name is one of the nodes,
// so a role read from one metric selects the same nodes in another.
func SeriesOnNodes(results []MetricResult, nodes []GPUNode) []MetricResult {
	names := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		names[n.Name] = struct{}{}
	}
	var out []MetricResult
	for _, r := range results {
		if _, ok := names[r.Labels.Resource["k8s.node.name"]]; ok {
			out = append(out, r)
		}
	}
	return out
}

// GPUAllocatable maps resource k8s.node.name to the GPUResource value of
// kube_node_status_allocatable series. Series of other resources are ignored. It
// returns an error for a series without a node name, a value that is not a GPU
// count, or two series of one node with different values.
func GPUAllocatable(results []MetricResult) (map[string]int, error) {
	out := make(map[string]int)
	for _, r := range results {
		if r.Labels.Datapoint["resource"] != GPUResource {
			continue
		}
		node := r.Labels.Resource["k8s.node.name"]
		if node == "" {
			return nil, fmt.Errorf("%s series for %s has no k8s.node.name", r.MetricName, GPUResource)
		}
		if r.Value < 0 || r.Value != math.Trunc(r.Value) {
			return nil, fmt.Errorf("node %s: %s allocatable %v is not a GPU count", node, GPUResource, r.Value)
		}
		n := int(r.Value)
		if prev, ok := out[node]; ok && prev != n {
			return nil, fmt.Errorf("node %s: %s allocatable is both %d and %d", node, GPUResource, prev, n)
		}
		out[node] = n
	}
	return out, nil
}
