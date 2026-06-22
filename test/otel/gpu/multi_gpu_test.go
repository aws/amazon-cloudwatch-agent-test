//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Multi-GPU tests validate that the g4dn.12xlarge node with 4 T4 GPUs
// produces distinct per-device metric series with correct device attributes.

package gpu

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	multiGpuInstanceType  = "g4dn.12xlarge"
	expectedMultiGPUCount = 4
)

func TestMultiGPUDeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	multiGPU := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multiGPU) > 0,
		"No DCGM_FI_DEV_GPU_UTIL results from %s node", multiGpuInstanceType)

	gpus := uniqueDatapointValues(multiGPU, "gpu")
	require.Equal(t, expectedMultiGPUCount, len(gpus), "Expected %d distinct gpu on %s, got %d: %v",
		expectedMultiGPUCount, multiGpuInstanceType, len(gpus), gpus)
}

func TestMultiGPUUniqueUUIDs(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	multiGPU := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multiGPU) > 0, "No results from %s node", multiGpuInstanceType)

	uuids := uniqueDatapointValues(multiGPU, "UUID")
	require.Equal(t, expectedMultiGPUCount, len(uuids), "Expected %d unique UUIDs, got %d", expectedMultiGPUCount, len(uuids))
	for _, uuid := range uuids {
		require.True(t, strings.HasPrefix(uuid, "GPU-"),
			"UUID should start with 'GPU-', got '%s'", uuid)
	}
}

func TestMultiGPUConsecutiveIndices(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	multiGPU := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multiGPU) > 0, "No results from %s node", multiGpuInstanceType)

	gpus := uniqueDatapointValues(multiGPU, "gpu")
	require.Equal(t, expectedMultiGPUCount, len(gpus), "Expected %d GPU indices, got %d: %v", expectedMultiGPUCount, len(gpus), gpus)
	for i := 0; i < expectedMultiGPUCount; i++ {
		expected := strconv.Itoa(i)
		found := false
		for _, g := range gpus {
			if g == expected {
				found = true
				break
			}
		}
		require.True(t, found, "Expected GPU index %d, got indices: %v", i, gpus)
	}
}

func TestMultiGPUAllMetricsPerDevice(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			multiGPU := filterByHostType(results, multiGpuInstanceType)
			require.True(t, len(multiGPU) > 0, "No %s results from %s node", metricName, multiGpuInstanceType)

			gpus := uniqueDatapointValues(multiGPU, "gpu")
			require.Equal(t, expectedMultiGPUCount, len(gpus), "%s: expected %d GPUs, got %d: %v",
				metricName, expectedMultiGPUCount, len(gpus), gpus)
		})
	}
}

func TestMultiGPUDeviceModelConsistency(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multiGPU := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multiGPU) > 0, "No results from %s node", multiGpuInstanceType)

	models := uniqueDatapointValues(multiGPU, "modelName")
	require.Equal(t, 1, len(models), "Expected 1 GPU model on %s, got %d: %v", multiGpuInstanceType, len(models), models)
}

func TestMultiGPUDriverVersionConsistency(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multiGPU := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multiGPU) > 0, "No results from %s node", multiGpuInstanceType)

	versions := uniqueDatapointValues(multiGPU, "DCGM_FI_DRIVER_VERSION")
	require.Equal(t, 1, len(versions), "Expected 1 driver version on %s, got %d: %v", multiGpuInstanceType, len(versions), versions)
}

// TestMultiGPUCorrelatedCount validates that exactly 1 GPU on the multi-GPU
// node is correlated to the multi-gpu-burn pod, and the rest are uncorrelated.
func TestMultiGPUCorrelatedCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multi := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiGpuInstanceType)

	var correlated, uncorrelated int
	for _, r := range multi {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "multi-gpu-burn") {
			correlated++
		} else if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
			uncorrelated++
		}
	}
	require.Equal(t, 1, correlated, "Expected 1 GPU correlated to multi-gpu-burn, got %d", correlated)
	require.Equal(t, expectedMultiGPUCount-1, uncorrelated, "Expected %d uncorrelated GPUs, got %d", expectedMultiGPUCount-1, uncorrelated)
}

// TestMultiGPUCorrelatedPodLabels validates that the correlated GPU result
// has the expected pod labels.
func TestMultiGPUCorrelatedPodLabels(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multi := filterByHostType(results, multiGpuInstanceType)

	for _, r := range multi {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "multi-gpu-burn") {
			require.Equal(t, "gpu-burn", r.Labels.Resource["k8s.container.name"], "multi-gpu-burn container name")
			require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "multi-gpu-burn namespace")
			require.Equal(t, "multi-gpu-burn", r.Labels.Resource["k8s.workload.name"], "multi-gpu-burn workload name")
			require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "multi-gpu-burn workload type")
			return
		}
	}
	t.Fatal("No DCGM_FI_DEV_GPU_UTIL result correlated to multi-gpu-burn pod")
}

// TestMultiGPUHostType validates the g4dn.12xlarge node reports host.type=g4dn.12xlarge
// and a valid EC2 instance ID.
func TestMultiGPUHostType(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	multi := filterByHostType(results, multiGpuInstanceType)
	require.NotEmpty(t, multi, "No DCGM results from %s node", multiGpuInstanceType)
	for _, r := range multi {
		require.Equal(t, multiGpuInstanceType, r.Labels.Resource["host.type"],
			"multi-GPU node host.type")
		hostID := r.Labels.Resource["host.id"]
		require.True(t, strings.HasPrefix(hostID, "i-"),
			"multi-GPU node host.id should start with 'i-', got %q", hostID)
	}
}
