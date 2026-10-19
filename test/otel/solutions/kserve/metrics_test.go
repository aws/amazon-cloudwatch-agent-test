//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package kserve

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// kserveMetrics are the kserve-controller-manager metrics expected in
// CloudWatch: controller-runtime reconcile and webhook health, and leader
// election. The webhook series exist once the InferenceService has been
// admitted.
var kserveMetrics = []otelmetrics.MetricDefinition{
	{Name: "controller_runtime_active_workers", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"controller"}},
	{Name: "controller_runtime_max_concurrent_reconciles", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"controller"}},
	{Name: "controller_runtime_reconcile_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"controller", "result"}, Unit: "1"},
	{Name: "controller_runtime_reconcile_errors_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"controller"}, Unit: "1"},
	{Name: "controller_runtime_reconcile_time_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"controller"}, Unit: "s"},
	{Name: "controller_runtime_webhook_requests_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"webhook", "code"}, Unit: "1"},
	{Name: "controller_runtime_webhook_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"webhook"}, Unit: "s"},
	{Name: "leader_election_master_status", MetricType: "gauge", Scope: otelmetrics.ScopeCluster, ExpectedLabels: []string{"name"}},
}
