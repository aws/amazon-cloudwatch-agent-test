// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// dcgmSeries is one fake DCGM series of a node's gpu, held by pod when pod is not empty.
func dcgmSeries(node, gpu, pod string) MetricResult {
	labels := NewMetricLabels()
	labels.Resource["k8s.node.name"] = node
	labels.Datapoint["gpu"] = gpu
	if pod != "" {
		labels.Resource["k8s.pod.name"] = pod
	}
	return MetricResult{MetricName: "DCGM_FI_DEV_GPU_UTIL", Labels: labels}
}

func nodeNames(nodes []GPUNode) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestReadGPURoles(t *testing.T) {
	t.Run("idle_node_and_busy_multi_gpu_node", func(t *testing.T) {
		roles := ReadGPURoles([]MetricResult{
			dcgmSeries("multi", "0", "multi-gpu-burn-abc"),
			dcgmSeries("multi", "1", ""),
			dcgmSeries("multi", "2", ""),
			dcgmSeries("multi", "3", ""),
			dcgmSeries("idle", "0", ""),
		}, "multi-gpu-burn")
		require.Equal(t, []string{"idle", "multi"}, nodeNames(roles.Nodes))
		require.Equal(t, []string{"idle"}, nodeNames(roles.Idle))
		require.Equal(t, []string{"multi"}, nodeNames(roles.Busy))
		require.Equal(t, []string{"0", "1", "2", "3"}, roles.Nodes[1].GPUs())
	})

	t.Run("single_gpu_nodes", func(t *testing.T) {
		roles := ReadGPURoles([]MetricResult{
			dcgmSeries("busy", "0", "multi-gpu-burn-abc"),
			dcgmSeries("idle", "0", ""),
		}, "multi-gpu-burn")
		require.Equal(t, []string{"idle"}, nodeNames(roles.Idle))
		require.Equal(t, []string{"busy"}, nodeNames(roles.Busy))
	})

	t.Run("other_pod_is_neither_idle_nor_busy", func(t *testing.T) {
		roles := ReadGPURoles([]MetricResult{dcgmSeries("other", "0", "training-xyz")}, "multi-gpu-burn")
		require.Equal(t, []string{"other"}, nodeNames(roles.Nodes))
		require.Empty(t, roles.Idle)
		require.Empty(t, roles.Busy)
	})

	t.Run("series_of_one_gpu_count_once", func(t *testing.T) {
		roles := ReadGPURoles([]MetricResult{
			dcgmSeries("node", "0", ""),
			dcgmSeries("node", "0", ""),
			dcgmSeries("node", "", ""),
		}, "multi-gpu-burn")
		require.Equal(t, []string{"0"}, roles.Nodes[0].GPUs(), "a repeated or empty gpu value is not a second GPU")
	})

	t.Run("series_without_node_name_are_left_out", func(t *testing.T) {
		roles := ReadGPURoles([]MetricResult{dcgmSeries("", "0", "")}, "multi-gpu-burn")
		require.Empty(t, roles.Nodes)
	})
}

func TestMultiGPUNodes(t *testing.T) {
	multi := GPUNode{Name: "multi", Series: []MetricResult{dcgmSeries("multi", "0", "")}}
	single := GPUNode{Name: "single", Series: []MetricResult{dcgmSeries("single", "0", "")}}
	nodes := []GPUNode{multi, single}

	got, err := MultiGPUNodes(nodes, map[string]int{"multi": 4, "single": 1, "other": 8})
	require.NoError(t, err)
	require.Equal(t, []string{"multi"}, nodeNames(got),
		"the allocatable decides, even when DCGM reports one gpu, and a node without DCGM series is not a GPU node")

	got, err = MultiGPUNodes(nodes, map[string]int{"single": 1})
	require.Error(t, err, "a GPU node with no allocatable must be an error, not a single-GPU node")
	require.Contains(t, err.Error(), "multi")
	require.Nil(t, got)

	got, err = MultiGPUNodes(nodes, map[string]int{"multi": 1, "single": 1})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSeriesOnNodes(t *testing.T) {
	a0, a1, b0 := dcgmSeries("a", "0", ""), dcgmSeries("a", "1", ""), dcgmSeries("b", "0", "")
	results := []MetricResult{a0, b0, a1}
	require.Equal(t, []MetricResult{a0, a1}, SeriesOnNodes(results, []GPUNode{{Name: "a"}}))
	require.Equal(t, results, SeriesOnNodes(results, []GPUNode{{Name: "a"}, {Name: "b"}}))
	require.Empty(t, SeriesOnNodes(results, nil))
}

// allocatableSeries is one fake kube_node_status_allocatable series.
func allocatableSeries(node, resource string, value float64) MetricResult {
	labels := NewMetricLabels()
	if node != "" {
		labels.Resource["k8s.node.name"] = node
	}
	labels.Datapoint["resource"] = resource
	return MetricResult{MetricName: "kube_node_status_allocatable", Labels: labels, Value: value}
}

func TestGPUAllocatable(t *testing.T) {
	got, err := GPUAllocatable([]MetricResult{
		allocatableSeries("multi", GPUResource, 4),
		allocatableSeries("multi", GPUResource, 4),
		allocatableSeries("multi", "cpu", 48),
		allocatableSeries("single", GPUResource, 1),
	})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"multi": 4, "single": 1}, got, "only the GPU resource counts, and an equal duplicate is fine")

	for name, results := range map[string][]MetricResult{
		"no_node_name":     {allocatableSeries("", GPUResource, 1)},
		"fractional":       {allocatableSeries("node", GPUResource, 1.5)},
		"negative":         {allocatableSeries("node", GPUResource, -1)},
		"conflicting_node": {allocatableSeries("node", GPUResource, 1), allocatableSeries("node", GPUResource, 4)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := GPUAllocatable(results)
			require.Error(t, err)
		})
	}
}
