//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Instance types in the standard cluster.
var clusterHostTypes = []string{"t3.medium"}

var clusterNodeGroups = []struct {
	InstanceType string
	Description  string
}{
	{"t3.medium", "standard"},
}

// --- Metric definitions (standard cluster only) ---

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

var controlPlaneMetrics = []otelmetrics.MetricDefinition{
	{Name: "apiserver_request_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"verb", "code"}, Unit: "1"},
	{Name: "apiserver_request_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"verb"}, Unit: "s"},
	{Name: "rest_client_requests_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"code", "method"}, Unit: "1"},
	{Name: "apiserver_current_inflight_requests", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"request_kind"}},
}

var ksmNodeScopedMetrics = []otelmetrics.MetricDefinition{
	{Name: "kube_node_status_condition", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"condition", "status"}},
	{Name: "kube_node_info", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kube_node_status_allocatable", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource", "unit"}},
	{Name: "kube_node_status_capacity", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource", "unit"}},
	{Name: "kube_pod_status_phase", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"phase"}},
	{Name: "kube_pod_container_status_running", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
}

var ksmClusterScopedMetrics = []otelmetrics.MetricDefinition{
	{Name: "kube_deployment_status_replicas_ready", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kube_deployment_status_replicas", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kube_daemonset_status_desired_number_scheduled", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kube_namespace_status_phase", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"phase"}},
}

// --- Aggregate slices ---

var daemonsetMetrics = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, nodeExporterMetrics...)
	all = append(all, cadvisorMetrics...)
	all = append(all, kubeletstatsMetrics...)
	return all
}()

var allMetrics = func() []otelmetrics.MetricDefinition {
	var all []otelmetrics.MetricDefinition
	all = append(all, daemonsetMetrics...)
	all = append(all, controlPlaneMetrics...)
	all = append(all, ksmNodeScopedMetrics...)
	all = append(all, ksmClusterScopedMetrics...)
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

func metricNamesByScope(defs []otelmetrics.MetricDefinition, scope otelmetrics.MetricScope) []string {
	var names []string
	for _, d := range defs {
		if d.Scope == scope {
			names = append(names, d.Name)
		}
	}
	return names
}

func daemonsetMetricNames() []string { return metricNames(daemonsetMetrics) }
func allMetricNames() []string       { return metricNames(allMetrics) }

func nodeMetricNames() []string {
	return metricNamesByScope(allMetrics, otelmetrics.ScopeNode)
}

func podMetricNames() []string {
	var names []string
	for _, m := range allMetrics {
		if m.Scope == otelmetrics.ScopePod || m.Scope == otelmetrics.ScopeContainer {
			names = append(names, m.Name)
		}
	}
	return names
}

func containerMetricNames() []string {
	return metricNamesByScope(allMetrics, otelmetrics.ScopeContainer)
}

func podScopedMetricNames() []string {
	names := daemonsetMetricNames()
	names = append(names, ksmPodBucket...)
	names = append(names, ksmContainerBucket...)
	return names
}

func hostEnrichedMetricNames() []string {
	return daemonsetMetricNames()
}

// nodeLabelEnrichedNames returns names of metrics that go through k8sattributes/node enrichment.
func nodeLabelEnrichedNames() []string { return hostEnrichedMetricNames() }

// prometheusScrapedNames returns names of all Prometheus-scraped metrics (excludes kubeletstats).
func prometheusScrapedNames() []string {
	var scraped []otelmetrics.MetricDefinition
	scraped = append(scraped, nodeExporterMetrics...)
	scraped = append(scraped, cadvisorMetrics...)
	scraped = append(scraped, controlPlaneMetrics...)
	return metricNames(scraped)
}

// podScopedCadvisorNames returns cadvisor metric names with ScopePod or ScopeContainer.
func podScopedCadvisorNames() []string {
	var names []string
	for _, m := range cadvisorMetrics {
		if m.Scope == otelmetrics.ScopePod || m.Scope == otelmetrics.ScopeContainer {
			names = append(names, m.Name)
		}
	}
	return names
}

// Instrumentation scope name constants.
const scopePrometheus = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"

// Custom node label used for node-color tests.
const nodeColorLabel = "k8s.node.label.ci-test.example.com/node-color"

// nodeColorToInstanceTypes maps node-color labels to expected instance types
// for this cluster. Each cluster type defines only its own colors.
// Phase 2 clusters add their colors (green=gpu, red=neuron, yellow=efa, white=attr-limit).
var nodeColorToInstanceTypes = map[string][]string{
	"blue": {"t3.medium"},
}
