//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpudriver

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// GPU node pools in the cluster; must match local.gpu_pools in terraform/azure/aks/main.tf.
const (
	idlePool = "gpuidle" // EKS gpu_single (g4dn.xlarge), idle
	busyPool = "gpuburn" // EKS gpu_multi (g4dn.12xlarge), runs multi-gpu-burn
)

// --- Metric definitions ---
//
// The 40 metrics the chart's own dcgm-exporter pods send on a T4 (37 DCGM_FI_* and 3 DCGM_EXP_*), as seen in
// CloudWatch on 2026-11-18 (runs 44284080397 and 44294122678). The six the EKS suite checks come first. The
// chart's field list also has NVLink error counters, row-remap counters and DCGM_FI_DEV_XID_ERROR, which this
// hardware does not report. Units and types are the ones CloudWatch reports; a metric with no unit has none.

var dcgmMetrics = []otelmetrics.MetricDefinition{
	// The six metrics the EKS gpu suite checks.
	{Name: "DCGM_FI_DEV_GPU_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_MEM_COPY_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_FB_USED", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_GPU_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_POWER_USAGE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_FB_FREE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	// derived metrics of the exporter (DCGM_EXP_*): health, clock events and XID errors
	{Name: "DCGM_EXP_CLOCK_EVENTS_COUNT", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1", ExpectedLabels: []string{"clock_event", "window_size_in_ms"}},
	{Name: "DCGM_EXP_GPU_HEALTH_STATUS", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1", ExpectedLabels: []string{"health_watch", "health_error_category", "health_error_code", "health_error_severity"}},
	{Name: "DCGM_EXP_XID_ERRORS_COUNT", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1", ExpectedLabels: []string{"window_size_in_ms"}},
	// power, temperature and memory
	{Name: "DCGM_FI_DEV_BOARD_POWER_LIMIT_ENFORCED_WATTS", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_BOARD_POWER_LIMIT_REQUESTED_WATTS", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_FB_TOTAL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_FB_USED_PERCENT", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "DCGM_FI_DEV_GPU_TEMP_SHUTDOWN_CELSIUS", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_GPU_TEMP_SLOWDOWN_CELSIUS", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_MEMORY_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	// clocks and energy
	{Name: "DCGM_FI_DEV_MEM_CLOCK", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MHz"},
	{Name: "DCGM_FI_DEV_SM_CLOCK", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MHz"},
	{Name: "DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "mJ"},
	// clock-event reasons, ECC and page-retirement counters
	{Name: "DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "ns"},
	{Name: "DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "ns"},
	{Name: "DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_THERM_SLOWDOWN_NS", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "ns"},
	{Name: "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PAGE_RETIRED_DBE_TOTAL", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PAGE_RETIRED_PENDING", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PAGE_RETIRED_SBE_TOTAL", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "1"},
	// PCIe link and replay
	{Name: "DCGM_FI_DEV_PCIE_LINK_GEN", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PCIE_LINK_WIDTH", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PCIE_MAX_LINK_GEN", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_DEV_PCIE_REPLAY_TOTAL", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "1"},
	// profiling (DCGM_FI_PROF_*)
	{Name: "DCGM_FI_PROF_DRAM_UTIL_RATIO", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_PROF_NVLINK_RX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By/s"},
	{Name: "DCGM_FI_PROF_NVLINK_TX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By/s"},
	{Name: "DCGM_FI_PROF_PCIE_RX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By/s"},
	{Name: "DCGM_FI_PROF_PCIE_TX_BYTES", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By/s"},
	{Name: "DCGM_FI_PROF_PIPE_TENSOR_ACTIVE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_PROF_SM_OCCUPANCY_RATIO", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "DCGM_FI_PROF_SM_UTIL_RATIO", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
}

// --- Helper functions ---

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// Custom pod label constant.
const podColorLabel = "k8s.pod.label.ci-test.example.com/pod-color"

// gpuPools lists the node pools with NVIDIA GPUs in this cluster.
var gpuPools = []struct {
	Pool        string
	Description string
}{
	{idlePool, "GPU-single"},
	{busyPool, "GPU-multi"},
}
