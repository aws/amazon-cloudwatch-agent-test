//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Node pool tests replace the EKS host.type tests: gpu-burn runs on the managed pool.
//
// The AKS host DCGM exporter on managed GPU pools has no pod mapping, so its series carry no pod,
// container or workload attribution even for the GPU used by multi-gpu-burn. These tests assert that
// absence, mirroring the EKS idle-node tests. If attribution appears, replace them with the EKS
// correlation checks (TestDCGMPodName, TestDCGMGpuburn*, TestDCGMHasRawPromotedKeys).

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

var managedDCGMMetricNamesList = metricNames(managedDCGMMetrics)

// TestDCGMManagedNodeNoPodName validates that managed GPU node results have no pod name or namespace.
func TestDCGMManagedNodeNoPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range managedDCGMMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			for _, r := range poolResults(t, metricName, managedPool) {
				for _, attr := range []string{"k8s.pod.name", "k8s.namespace.name"} {
					_, has := r.Labels.Resource[attr]
					require.False(t, has,
						"%s managed GPU node should not have %s but got: %s", metricName, attr, r.Labels.Resource[attr])
				}
			}
		})
	}
}

// TestDCGMManagedNodeNoContainerName validates that managed GPU node results do NOT have k8s.container.name.
func TestDCGMManagedNodeNoContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range managedDCGMMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			for _, r := range poolResults(t, metricName, managedPool) {
				_, has := r.Labels.Resource["k8s.container.name"]
				require.False(t, has,
					"%s managed GPU node should not have k8s.container.name but got: %s",
					metricName, r.Labels.Resource["k8s.container.name"])
			}
		})
	}
}

// TestDCGMManagedNodeNoWorkloadLabels validates that managed GPU node results do NOT have workload labels.
func TestDCGMManagedNodeNoWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range managedDCGMMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			for _, r := range poolResults(t, metricName, managedPool) {
				for _, attr := range []string{"k8s.workload.name", "k8s.workload.type", "k8s.deployment.name"} {
					_, has := r.Labels.Resource[attr]
					require.False(t, has,
						"%s managed GPU node has unexpected %s: %s", metricName, attr, r.Labels.Resource[attr])
				}
			}
		})
	}
}

// TestDCGMManagedNodeNoPodColor validates that managed GPU node results do NOT have the pod color label.
func TestDCGMManagedNodeNoPodColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range managedDCGMMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			for _, r := range poolResults(t, metricName, managedPool) {
				_, has := r.Labels.Resource[podColorLabel]
				require.False(t, has,
					"%s managed GPU node should not have %s", metricName, podColorLabel)
			}
		})
	}
}

// TestDCGMManagedNodeNoRawPromotedKeys validates that the raw Prometheus keys (pod, namespace,
// container) are absent at datapoint scope; the host exporter does not emit them.
func TestDCGMManagedNodeNoRawPromotedKeys(t *testing.T) {
	t.Parallel()
	for _, metricName := range managedDCGMMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			for _, r := range poolResults(t, metricName, managedPool) {
				for _, key := range []string{"pod", "namespace", "container"} {
					_, has := r.Labels.Datapoint[key]
					require.False(t, has,
						"%s managed GPU node should not have datapoint '%s' but got: %s", metricName, key, r.Labels.Datapoint[key])
				}
			}
		})
	}
}

// TestDCGMManagedActiveNonzero validates that the managed GPU node has non-zero utilization
// (multi-gpu-burn runs on it).
func TestDCGMManagedActiveNonzero(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	results = onPool(results, managedPool)
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available from %s (no GPU nodes?)", managedPool)

	hasNonzero := false
	for _, r := range results {
		if r.Value > 0 {
			hasNonzero = true
			break
		}
	}
	require.True(t, hasNonzero, "All %s GPU utilization values are zero — is multi-gpu-burn running?", managedPool)
}
