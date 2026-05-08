//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Instance types in the GPU cluster.
var clusterHostTypes = []string{"g4dn.xlarge", "g4dn.12xlarge"}

// --- Metric definitions ---

var nodeExporterMetrics = []otelmetrics.MetricDefinition{
	{Name: "node_cpu_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopeNode, ExpectedLabels: []string{"cpu", "mode"}, Unit: "s"},
	{Name: "node_memory_MemAvailable_bytes", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "By"},
	{Name: "node_filesystem_avail_bytes", MetricType: "gauge", Scope: otelmetrics.ScopeNode, ExpectedLabels: []string{"device", "mountpoint", "fstype"}, Unit: "By"},
	{Name: "node_network_receive_bytes_total", MetricType: "counter", Scope: otelmetrics.ScopeNode, ExpectedLabels: []string{"device"}, Unit: "By"},
	{Name: "node_load1", MetricType: "gauge", Scope: otelmetrics.ScopeNode},
}

var cadvisorMetrics = []otelmetrics.MetricDefinition{
	{Name: "container_cpu_usage_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopeContainer, ExpectedLabels: []string{"cpu"}, Unit: "s"},
	{Name: "container_memory_working_set_bytes", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
	{Name: "container_memory_usage_bytes", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
	{Name: "container_network_receive_bytes_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"interface"}, Unit: "By"},
}

var kubeletstatsNodeMetrics = []otelmetrics.MetricDefinition{
	{Name: "k8s.node.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "1"},
	{Name: "k8s.node.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "By"},
	{Name: "k8s.node.filesystem.available", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "By"},
	{Name: "k8s.node.network.io", MetricType: "counter", Scope: otelmetrics.ScopeNode, ExpectedLabels: []string{"interface", "direction"}, Unit: "By"},
}

var kubeletstatsPodMetrics = []otelmetrics.MetricDefinition{
	{Name: "k8s.pod.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "k8s.pod.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "k8s.pod.network.io", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"interface", "direction"}, Unit: "By"},
}

var kubeletstatsContainerMetrics = []otelmetrics.MetricDefinition{
	{Name: "container.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "1"},
	{Name: "container.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
	{Name: "container.memory.usage", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
}

var kubeletstatsMetrics = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, kubeletstatsNodeMetrics...)
	all = append(all, kubeletstatsPodMetrics...)
	all = append(all, kubeletstatsContainerMetrics...)
	return all
}()

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

// gpuInstanceTypes lists instance types with NVIDIA GPUs in this cluster.
var gpuInstanceTypes = []struct {
	InstanceType string
	Description  string
}{
	{"g4dn.xlarge", "GPU-single"},
	{"g4dn.12xlarge", "GPU-multi"},
}
