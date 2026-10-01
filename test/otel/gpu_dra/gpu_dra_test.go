//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package gpu_dra validates per-device GPU pod correlation on the Dynamic Resource
// Allocation (DRA) path — the counterpart of the device-plugin `gpu` package's
// multi_gpu tests. GPUs are allocated via the NVIDIA DRA driver (DeviceClass
// gpu.nvidia.com, driver gpu.nvidia.com) through a ResourceClaimTemplate instead of
// the nvidia.com/gpu device-plugin resource. The emitted DCGM metrics are identical,
// so the assertions mirror the device-plugin multi_gpu_test.
//
// Cluster topology:
//   - 1x g4dn.12xlarge = 4 T4 GPUs
//   - GPUs exposed via the NVIDIA DRA driver, not the device plugin
//   - A burn workload (gpu-burn-dra) claims 1 GPU via DRA
package gpu_dra

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	multiGpuInstanceType  = "g4dn.12xlarge"
	expectedMultiGPUCount = 4

	// burnPodPrefix is the DRA burn workload that claims 1 GPU via a ResourceClaimTemplate.
	burnPodPrefix        = "gpu-burn-dra"
	expectedClaimedGPUs  = 1
	expectedUncorrelated = expectedMultiGPUCount - expectedClaimedGPUs
)

// TestGPUDRADeviceCount validates the node exposes the expected number of GPUs.
func TestGPUDRADeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available")

	multi := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multi) > 0, "No DCGM_FI_DEV_GPU_UTIL results from %s node", multiGpuInstanceType)

	gpus := uniqueDatapointValues(multi, "gpu")
	require.Equal(t, expectedMultiGPUCount, len(gpus),
		"Expected %d distinct gpu on %s, got %d: %v", expectedMultiGPUCount, multiGpuInstanceType, len(gpus), gpus)
}

// TestGPUDRAConsecutiveIndices validates GPU indices are 0..N-1.
func TestGPUDRAConsecutiveIndices(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multi := filterByHostType(results, multiGpuInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiGpuInstanceType)

	gpus := uniqueDatapointValues(multi, "gpu")
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

// TestGPUDRAAllMetricsPerDevice validates every DCGM metric reports for all 4 GPUs.
func TestGPUDRAAllMetricsPerDevice(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			multi := filterByHostType(results, multiGpuInstanceType)
			require.True(t, len(multi) > 0, "No %s results from %s node", metricName, multiGpuInstanceType)

			gpus := uniqueDatapointValues(multi, "gpu")
			require.Equal(t, expectedMultiGPUCount, len(gpus),
				"%s: expected %d GPUs, got %d: %v", metricName, expectedMultiGPUCount, len(gpus), gpus)
		})
	}
}

// TestGPUDRAClaimedVsUnclaimedCorrelation is the DRA-path per-device correlation
// guard. On a 4-GPU node, the burn pod claims exactly 1 GPU via DRA, so exactly 1
// GPU correlates to that pod and the remaining 3 carry NO pod. This fails if DRA
// correlation collapses GPUs onto one pod, over-correlates unclaimed GPUs, or maps
// a GPU to the wrong pod.
func TestGPUDRAClaimedVsUnclaimedCorrelation(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	multi := filterByHostType(results, multiGpuInstanceType)
	require.NotEmpty(t, multi, "No DCGM_FI_DEV_GPU_UTIL results from %s node", multiGpuInstanceType)

	// For each GPU, collect the distinct pods it is attributed to (empty = unclaimed).
	gpuPods := make(map[string]map[string]struct{})
	for _, r := range multi {
		r := r
		gpu := r.Labels.Datapoint["gpu"]
		if gpu == "" {
			continue
		}
		if gpuPods[gpu] == nil {
			gpuPods[gpu] = make(map[string]struct{})
		}
		if pod := r.Labels.Resource["k8s.pod.name"]; pod != "" {
			gpuPods[gpu][pod] = struct{}{}
		}
	}
	require.Len(t, gpuPods, expectedMultiGPUCount,
		"expected %d GPUs on the node, got %d", expectedMultiGPUCount, len(gpuPods))

	var claimed, unclaimed []string
	for gpu, pods := range gpuPods {
		switch len(pods) {
		case 0:
			unclaimed = append(unclaimed, gpu)
		case 1:
			var pod string
			for p := range pods {
				pod = p
			}
			require.True(t, strings.HasPrefix(pod, burnPodPrefix),
				"GPU %s correlated to unexpected pod %q (expected %s*)", gpu, pod, burnPodPrefix)
			claimed = append(claimed, gpu)
		default:
			t.Errorf("GPU %s correlated to multiple pods", gpu)
		}
	}

	require.Len(t, claimed, expectedClaimedGPUs,
		"expected %d claimed GPU correlated to %s*, got %d: %v "+
			"(collapse over-correlates unclaimed GPUs onto a pod)",
		expectedClaimedGPUs, burnPodPrefix, len(claimed), claimed)
	require.Len(t, unclaimed, expectedUncorrelated,
		"expected %d uncorrelated GPUs with no pod, got %d: %v",
		expectedUncorrelated, len(unclaimed), unclaimed)
}

// TestGPUDRABurnPodLabels validates the correlated GPU's pod labels via a targeted
// PromQL query (avoids stale series in the shared OTLP store).
func TestGPUDRABurnPodLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	promql := fmt.Sprintf(
		`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"%s.*"}`,
		escapePromQL(cfg.ClusterName), burnPodPrefix)
	burn, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL for %s", burnPodPrefix)
	require.Equal(t, expectedClaimedGPUs, len(burn),
		"Expected %d GPU correlated to %s, got %d", expectedClaimedGPUs, burnPodPrefix, len(burn))

	for _, r := range burn {
		r := r
		require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s namespace", burnPodPrefix)
	}
}
