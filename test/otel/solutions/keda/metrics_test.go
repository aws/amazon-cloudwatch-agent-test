//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package keda

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// kedaMetrics defines the KEDA controller metrics expected in CloudWatch.
// These are Prometheus metrics scraped from the KEDA operator pods and
// forwarded by the CloudWatch Agent's KEDA integration pipeline.
var kedaMetrics = []otelmetrics.MetricDefinition{
	// Build info
	{Name: "keda_build_info", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},

	// Scaler metrics
	{Name: "keda_scaler_metrics_value", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"scaler", "scaledObject", "metric"}},
	{Name: "keda_scaler_metrics_latency_seconds", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"scaler", "scaledObject", "metric"}, Unit: "s"},
	{Name: "keda_scaler_active", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"scaler", "scaledObject", "metric"}},
	{Name: "keda_scaler_errors_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"scaler", "scaledObject"}, Unit: "1"},

	// Scaled object metrics
	{Name: "keda_scaled_object_paused", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"scaledObject", "namespace"}},

	// Controller metrics
	{Name: "keda_internal_scale_loop_latency_seconds", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"type", "resource"}, Unit: "s"},

	// Resource totals
	{Name: "keda_resource_totals", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"type", "namespace", "resource"}},
}

func kedaMetricNames() []string {
	names := make([]string, len(kedaMetrics))
	for i, d := range kedaMetrics {
		names[i] = d.Name
	}
	return names
}
