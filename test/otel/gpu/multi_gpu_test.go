//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Multi-GPU tests validate that a node whose nvidia.com/gpu allocatable is more
// than 1 produces one series per device, for every device in the allocatable,
// with correct device attributes.

package gpu

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

func TestMultiGPUDeviceCount(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		want := allocatableGPUs(t, node.Name)
		gpus := node.GPUs()
		require.Equal(t, want, len(gpus), "Expected %d distinct gpu on %s (nvidia.com/gpu allocatable), got %d: %v",
			want, node.Name, len(gpus), gpus)
	}
}

func TestMultiGPUUniqueUUIDs(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		want := allocatableGPUs(t, node.Name)
		uuids := uniqueDatapointValues(node.Series, "UUID")
		require.Equal(t, want, len(uuids), "Expected %d unique UUIDs on %s, got %d", want, node.Name, len(uuids))
		for _, uuid := range uuids {
			require.True(t, strings.HasPrefix(uuid, "GPU-"),
				"UUID should start with 'GPU-', got '%s'", uuid)
		}
	}
}

func TestMultiGPUConsecutiveIndices(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		want := allocatableGPUs(t, node.Name)
		gpus := node.GPUs()
		require.Equal(t, want, len(gpus), "Expected %d GPU indices on %s, got %d: %v", want, node.Name, len(gpus), gpus)
		for i := 0; i < want; i++ {
			expected := strconv.Itoa(i)
			found := false
			for _, g := range gpus {
				if g == expected {
					found = true
					break
				}
			}
			require.True(t, found, "Expected GPU index %d on %s, got indices: %v", i, node.Name, gpus)
		}
	}
}

func TestMultiGPUAllMetricsPerDevice(t *testing.T) {
	t.Parallel()
	nodes := multiGPUNodes(t)
	want := make(map[string]int, len(nodes))
	for _, node := range nodes {
		want[node.Name] = allocatableGPUs(t, node.Name)
	}
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			for _, node := range nodes {
				multiGPU := otelmetrics.SeriesOnNodes(results, []otelmetrics.GPUNode{node})
				require.True(t, len(multiGPU) > 0, "No %s results from %s", metricName, node.Name)

				gpus := uniqueDatapointValues(multiGPU, "gpu")
				require.Equal(t, want[node.Name], len(gpus), "%s on %s: expected %d GPUs, got %d: %v",
					metricName, node.Name, want[node.Name], len(gpus), gpus)
			}
		})
	}
}

func TestMultiGPUDeviceModelConsistency(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		models := uniqueDatapointValues(node.Series, "modelName")
		require.Equal(t, 1, len(models), "Expected 1 GPU model on %s, got %d: %v", node.Name, len(models), models)
	}
}

func TestMultiGPUDriverVersionConsistency(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		versions := uniqueDatapointValues(node.Series, "DCGM_FI_DRIVER_VERSION")
		require.Equal(t, 1, len(versions), "Expected 1 driver version on %s, got %d: %v", node.Name, len(versions), versions)
	}
}

// TestMultiGPUCorrelatedCount validates that exactly 1 GPU on the multi-GPU
// node that runs multi-gpu-burn is correlated to the pod, which requests one
// GPU, and the rest are uncorrelated.
func TestMultiGPUCorrelatedCount(t *testing.T) {
	t.Parallel()
	for _, node := range busyMultiGPUNodes(t) {
		var correlated, uncorrelated int
		for _, r := range node.Series {
			if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], gpuBurnPodPrefix) {
				correlated++
			} else if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
				uncorrelated++
			}
		}
		want := allocatableGPUs(t, node.Name) - 1
		require.Equal(t, 1, correlated, "Expected 1 GPU on %s correlated to multi-gpu-burn, got %d", node.Name, correlated)
		require.Equal(t, want, uncorrelated, "Expected %d uncorrelated GPUs on %s, got %d", want, node.Name, uncorrelated)
	}
}

// TestMultiGPUCorrelatedPodLabels validates that the correlated GPU result
// has the expected pod labels.
func TestMultiGPUCorrelatedPodLabels(t *testing.T) {
	t.Parallel()
	for _, node := range busyMultiGPUNodes(t) {
		for _, r := range node.Series {
			if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], gpuBurnPodPrefix) {
				require.Equal(t, "gpu-burn", r.Labels.Resource["k8s.container.name"], "multi-gpu-burn container name")
				require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "multi-gpu-burn namespace")
				require.Equal(t, "multi-gpu-burn", r.Labels.Resource["k8s.workload.name"], "multi-gpu-burn workload name")
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "multi-gpu-burn workload type")
				return
			}
		}
	}
	t.Fatal("No DCGM_FI_DEV_GPU_UTIL result correlated to multi-gpu-burn pod")
}

// TestMultiGPUHostType validates that every series of the multi-GPU node reports
// one host.type of the configured host types and a valid EC2 instance ID.
func TestMultiGPUHostType(t *testing.T) {
	t.Parallel()
	for _, node := range multiGPUNodes(t) {
		hostType := node.Series[0].Labels.Resource["host.type"]
		require.Contains(t, clusterHostTypes, hostType, "multi-GPU node %s host.type", node.Name)
		for _, r := range node.Series {
			require.Equal(t, hostType, r.Labels.Resource["host.type"],
				"multi-GPU node %s host.type", node.Name)
			hostID := r.Labels.Resource["host.id"]
			require.True(t, strings.HasPrefix(hostID, "i-"),
				"multi-GPU node %s host.id should start with 'i-', got %q", node.Name, hostID)
		}
	}
}
