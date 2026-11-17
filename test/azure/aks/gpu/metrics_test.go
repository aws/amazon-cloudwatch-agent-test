//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// Node pool in the GPU cluster; must match local.gpu_pools in terraform/azure/aks/main.tf.
// AKS-managed GPU: host DCGM on :19400, runs multi-gpu-burn.
const managedPool = "gpumanaged"

// --- Metric definitions ---

// managedDCGMMetrics are served by the AKS host DCGM exporter on managed-GPU pools. The chart's
// dcgm-host pipeline has no metric filter, so all of them are exported. Units are the ones the chart's
// transform/cw_k8s_ci_v0_set_unit maps; the other fields get none.
var managedDCGMMetrics = []otelmetrics.MetricDefinition{
	{Name: "DCGM_FI_DEV_DEC_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_ENC_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_FB_FREE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_FB_RESERVED", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_FB_USED", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_GPU_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_GPU_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_MEMORY_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_MEM_CLOCK", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_MEM_COPY_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_PCIE_REPLAY_COUNTER", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_POWER_USAGE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_SM_CLOCK", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MHz"},
	{Name: "DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_VGPU_LICENSE_STATUS", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_PROF_DRAM_ACTIVE", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_PROF_GR_ENGINE_ACTIVE", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_PROF_PCIE_RX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_PROF_PCIE_TX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_PROF_PIPE_TENSOR_ACTIVE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
}

var poolMetrics = map[string][]otelmetrics.MetricDefinition{
	managedPool: managedDCGMMetrics,
}

// Custom labels applied by the Terraform stack.
const (
	podColorLabel  = "k8s.pod.label.ci-test.example.com/pod-color"
	nodeColorLabel = "k8s.node.label.ci-test.example.com/node-color"
)

// --- Helper functions ---

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// onPool returns the results whose node belongs to the given AKS node pool
// (AKS node names are aks-<pool>-<id>-vmss<n>). Replaces filterByHostType from test/otel/gpu.
func onPool(results []otelmetrics.MetricResult, pool string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if strings.HasPrefix(r.Labels.Resource["k8s.node.name"], "aks-"+pool+"-") {
			out = append(out, r)
		}
	}
	return out
}

// poolResults queries a metric and returns its results from one pool, failing if there are none.
func poolResults(t *testing.T, metricName, pool string) []otelmetrics.MetricResult {
	t.Helper()
	results, err := queryCache.Get(context.Background(), metricName)
	require.NoError(t, err, "querying %s", metricName)
	filtered := onPool(results, pool)
	require.NotEmpty(t, filtered, "%s: no results from %s nodes (%d results in the cluster)", metricName, pool, len(results))
	return filtered
}

// forEachPoolMetric runs fn as a parallel subtest <pool>/<metric> for every expected metric of every pool.
func forEachPoolMetric(t *testing.T, fn func(t *testing.T, pool, metricName string, results []otelmetrics.MetricResult)) {
	t.Helper()
	pools := make([]string, 0, len(poolMetrics))
	for p := range poolMetrics {
		pools = append(pools, p)
	}
	sort.Strings(pools)
	for _, pool := range pools {
		for _, metricName := range metricNames(poolMetrics[pool]) {
			pool, metricName := pool, metricName
			t.Run(pool+"/"+metricName, func(t *testing.T) {
				t.Parallel()
				fn(t, pool, metricName, poolResults(t, metricName, pool))
			})
		}
	}
}

// TestDCGMMetricsPresent validates every expected metric arrives from its pool.
func TestDCGMMetricsPresent(t *testing.T) {
	t.Parallel()
	forEachPoolMetric(t, func(*testing.T, string, string, []otelmetrics.MetricResult) {})
}
