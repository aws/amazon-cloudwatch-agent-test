//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// gpuBurnPodPrefix names the pods of the multi-gpu-burn Deployment, which holds one GPU on the busy node.
const gpuBurnPodPrefix = "multi-gpu-burn"

// gpuRoles reads the node roles from DCGM_FI_DEV_GPU_UTIL, so every test sees the same nodes.
func gpuRoles(t *testing.T) otelmetrics.GPURoles {
	t.Helper()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")
	return otelmetrics.ReadGPURoles(results, gpuBurnPodPrefix)
}

// idleNodes returns the GPU nodes where no pod holds a GPU.
func idleNodes(t *testing.T) []otelmetrics.GPUNode {
	t.Helper()
	nodes := gpuRoles(t).Idle
	require.NotEmpty(t, nodes, "No idle GPU node: every node of host types %v has DCGM series with k8s.pod.name", clusterHostTypes)
	return nodes
}

// busyNodes returns the GPU nodes where multi-gpu-burn holds a GPU.
func busyNodes(t *testing.T) []otelmetrics.GPUNode {
	t.Helper()
	nodes := gpuRoles(t).Busy
	require.NotEmpty(t, nodes, "No GPU node of host types %v runs %s", clusterHostTypes, gpuBurnPodPrefix)
	return nodes
}

// multiGPUNodes returns the GPU nodes whose nvidia.com/gpu allocatable is more than 1.
// With none, it skips the test when OTEL_HOST_TYPES overrides the defaults and fails it otherwise.
func multiGPUNodes(t *testing.T) []otelmetrics.GPUNode {
	t.Helper()
	nodes, err := otelmetrics.MultiGPUNodes(gpuRoles(t).Nodes, gpuAllocatable(t))
	require.NoError(t, err)
	requireMultiGPU(t, len(nodes) > 0, "No GPU node of host types %v has an nvidia.com/gpu allocatable above 1", clusterHostTypes)
	return nodes
}

// busyMultiGPUNodes returns the multi-GPU nodes where multi-gpu-burn holds a GPU, and skips or
// fails like multiGPUNodes when there are none.
func busyMultiGPUNodes(t *testing.T) []otelmetrics.GPUNode {
	t.Helper()
	var nodes []otelmetrics.GPUNode
	for _, n := range multiGPUNodes(t) {
		if n.Runs(gpuBurnPodPrefix) {
			nodes = append(nodes, n)
		}
	}
	requireMultiGPU(t, len(nodes) > 0, "%s runs on no multi-GPU node of host types %v", gpuBurnPodPrefix, clusterHostTypes)
	return nodes
}

// requireMultiGPU skips the test when ok is false and OTEL_HOST_TYPES overrides the defaults,
// because such a cluster can have no multi-GPU node. Otherwise it requires ok.
func requireMultiGPU(t *testing.T, ok bool, format string, args ...interface{}) {
	t.Helper()
	if !ok && hostTypesOverridden {
		t.Skipf(format+" (%s overrides the default host types %v)", append(args, otelmetrics.HostTypesEnvVar, defaultHostTypes)...)
	}
	require.Truef(t, ok, format, args...)
}

// nodeNames returns the names of the nodes, for messages.
func nodeNames(nodes []otelmetrics.GPUNode) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	return names
}

var (
	gpuAllocatableByNode map[string]int
	gpuAllocatableOnce   sync.Once
	gpuAllocatableErr    error
)

// gpuAllocatable maps node names to their nvidia.com/gpu allocatable from kube_node_status_allocatable.
// It reads through allNodesCache, which pins no host types, because kube-state-metrics series are cluster-scoped.
func gpuAllocatable(t *testing.T) map[string]int {
	t.Helper()
	gpuAllocatableOnce.Do(func() {
		var results []otelmetrics.MetricResult
		results, gpuAllocatableErr = allNodesCache.GetWithFilter(context.Background(), "kube_node_status_allocatable",
			map[string]string{"resource": otelmetrics.GPUResource})
		if gpuAllocatableErr == nil {
			gpuAllocatableByNode, gpuAllocatableErr = otelmetrics.GPUAllocatable(results)
		}
	})
	require.NoError(t, gpuAllocatableErr, "reading kube_node_status_allocatable{resource=%q}", otelmetrics.GPUResource)
	return gpuAllocatableByNode
}

// allocatableGPUs returns the node's nvidia.com/gpu allocatable.
func allocatableGPUs(t *testing.T, node string) int {
	t.Helper()
	n, ok := gpuAllocatable(t)[node]
	require.True(t, ok, "No kube_node_status_allocatable{resource=%q} series for node %s", otelmetrics.GPUResource, node)
	return n
}

// uniqueDatapointValues returns the sorted unique non-empty values of a
// datapoint-level attribute across all results.
func uniqueDatapointValues(results []otelmetrics.MetricResult, attr string) []string {
	seen := make(map[string]struct{})
	for _, r := range results {
		if v, ok := r.Labels.Datapoint[attr]; ok && v != "" {
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
