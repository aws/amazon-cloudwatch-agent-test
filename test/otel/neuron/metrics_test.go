//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Custom pod label constant.
const podColorLabel = "k8s.pod.label.ci-test.example.com/pod-color"

// --- Standard metric definitions (reused from standard cluster) ---

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

var kubeletstatsMetrics = []otelmetrics.MetricDefinition{
	{Name: "k8s.node.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "1"},
	{Name: "k8s.node.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "By"},
	{Name: "k8s.node.filesystem.available", MetricType: "gauge", Scope: otelmetrics.ScopeNode, Unit: "By"},
	{Name: "k8s.node.network.io", MetricType: "counter", Scope: otelmetrics.ScopeNode, ExpectedLabels: []string{"interface", "direction"}, Unit: "By"},
	{Name: "k8s.pod.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "k8s.pod.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "k8s.pod.network.io", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"interface", "direction"}, Unit: "By"},
	{Name: "container.cpu.utilization", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "1"},
	{Name: "container.memory.working_set", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
	{Name: "container.memory.usage", MetricType: "gauge", Scope: otelmetrics.ScopeContainer, Unit: "By"},
}

// --- Neuron metric definitions ---

var neuronMetrics = []otelmetrics.MetricDefinition{
	{Name: "neuron_runtime_memory_used_bytes", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"memory_location"}, Unit: "By"},
	{Name: "neuroncore_utilization_ratio", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "neuroncore_memory_usage_model_shared_scratchpad", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "execution_latency_seconds", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"percentile"}, Unit: "s"},
}

// neuronCoreLevelMetrics are per-NeuronCore metrics (have aws.neuron.device + aws.neuron.core).
var neuronCoreLevelMetrics = []otelmetrics.MetricDefinition{
	{Name: "neuroncore_utilization_ratio", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "neuroncore_memory_usage_model_shared_scratchpad", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By"},
}

// neuronRuntimeLevelMetrics are per-runtime metrics (NOT per-core).
var neuronRuntimeLevelMetrics = []otelmetrics.MetricDefinition{
	{Name: "neuron_runtime_memory_used_bytes", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"memory_location"}, Unit: "By"},
	{Name: "execution_latency_seconds", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"percentile"}, Unit: "s"},
}

// --- Aggregate slices ---

var daemonsetMetrics = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, nodeExporterMetrics...)
	all = append(all, cadvisorMetrics...)
	all = append(all, kubeletstatsMetrics...)
	return all
}()

// --- Helper functions ---

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func neuronMetricNames() []string    { return metricNames(neuronMetrics) }
func daemonsetMetricNames() []string { return metricNames(daemonsetMetrics) }

func allMetricNames() []string {
	var all []otelmetrics.MetricDefinition
	all = append(all, daemonsetMetrics...)
	all = append(all, neuronMetrics...)
	return metricNames(all)
}
