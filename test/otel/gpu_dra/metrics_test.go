//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu_dra

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// The DRA cluster has a single multi-GPU node (g4dn.12xlarge, 4 GPUs).
var clusterHostTypes = []string{multiGpuInstanceType}

// DCGM metric definitions (same source names as the device-plugin gpu package).
var dcgmMetrics = []otelmetrics.MetricDefinition{
	{Name: "DCGM_FI_DEV_GPU_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_MEM_COPY_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_FB_USED", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_GPU_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_POWER_USAGE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_FB_FREE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
}

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

var dcgmMetricNamesList = metricNames(dcgmMetrics)
