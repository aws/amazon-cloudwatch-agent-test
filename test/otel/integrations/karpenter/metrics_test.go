//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package karpenter

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// karpenterMetrics defines the Karpenter controller metrics expected in CloudWatch.
// These are Prometheus metrics scraped from the Karpenter controller pods and
// forwarded by the CloudWatch Agent's Karpenter integration pipeline.
var karpenterMetrics = []otelmetrics.MetricDefinition{
	// Build info
	{Name: "karpenter_build_info", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},

	// Pod scheduling metrics
	{Name: "karpenter_pods_state", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"phase"}},
	{Name: "karpenter_pods_bound_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, Unit: "s"},

	// Scheduler metrics
	{Name: "karpenter_scheduler_ignored_pods_count", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "karpenter_scheduler_scheduling_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, Unit: "s"},

	// Node resource metrics
	{Name: "karpenter_nodes_allocatable", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_total_daemon_requests", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_total_daemon_limits", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_total_pod_requests", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_total_pod_limits", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_system_overhead", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodes_current_lifetime_seconds", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, Unit: "s"},

	// Cluster state metrics
	{Name: "karpenter_cluster_state_node_count", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "karpenter_cluster_state_synced", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "karpenter_cluster_state_unsynced_time_seconds", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, Unit: "s"},
	{Name: "karpenter_cluster_utilization_percent", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},

	// NodePool metrics
	{Name: "karpenter_nodepools_limit", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},
	{Name: "karpenter_nodepools_usage", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"resource_type"}},

	// Cloudprovider metrics
	{Name: "karpenter_cloudprovider_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"method"}, Unit: "s"},
	{Name: "karpenter_cloudprovider_instance_type_cpu_cores", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "karpenter_cloudprovider_instance_type_memory_bytes", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, Unit: "By"},
	{Name: "karpenter_cloudprovider_instance_type_offering_available", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "karpenter_cloudprovider_instance_type_offering_price_estimate", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},

	// Disruption metrics
	{Name: "karpenter_voluntary_disruption_eligible_nodes", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"reason"}},
	{Name: "karpenter_voluntary_disruption_consolidation_timeouts_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, Unit: "1"},
	{Name: "karpenter_voluntary_disruption_decision_evaluation_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, Unit: "s"},
}

func karpenterMetricNames() []string {
	names := make([]string, len(karpenterMetrics))
	for i, d := range karpenterMetrics {
		names[i] = d.Name
	}
	return names
}
