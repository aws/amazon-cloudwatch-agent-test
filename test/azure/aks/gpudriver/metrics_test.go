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

var dcgmMetrics = []otelmetrics.MetricDefinition{
	{Name: "DCGM_FI_DEV_GPU_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_MEM_COPY_UTIL", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "%"},
	{Name: "DCGM_FI_DEV_FB_USED", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
	{Name: "DCGM_FI_DEV_GPU_TEMP", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "Cel"},
	{Name: "DCGM_FI_DEV_POWER_USAGE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "W"},
	{Name: "DCGM_FI_DEV_FB_FREE", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "MiBy"},
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
