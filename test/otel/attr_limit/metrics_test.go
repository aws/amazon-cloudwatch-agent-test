//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package attr_limit

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

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

var daemonsetMetrics = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, nodeExporterMetrics...)
	all = append(all, cadvisorMetrics...)
	all = append(all, kubeletstatsMetrics...)
	return all
}()

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func daemonsetMetricNames() []string { return metricNames(daemonsetMetrics) }

func podMetricNames() []string {
	var names []string
	for _, m := range daemonsetMetrics {
		m := m
		if m.Scope == otelmetrics.ScopePod || m.Scope == otelmetrics.ScopeContainer {
			names = append(names, m.Name)
		}
	}
	return names
}
