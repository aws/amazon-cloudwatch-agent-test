//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package knative

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Knative Serving >= 1.19 names (OpenTelemetry SDK, kn_* prefix).

// controlPlaneMetrics come from the autoscaler, activator, controller and
// webhook, scraped by the cluster-scraper.
var controlPlaneMetrics = []otelmetrics.MetricDefinition{
	{Name: "kn_workqueue_depth", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kn_workqueue_adds_total", MetricType: "counter", Scope: otelmetrics.ScopeCluster, Unit: "1"},
	{Name: "kn_revision_pods_desired", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kn_revision_pods_count", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kn_revision_request_concurrency", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kn_revision_concurrency_target", MetricType: "gauge", Scope: otelmetrics.ScopeCluster},
	{Name: "kn_autoscaler_scrape_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, Unit: "s"},
	{Name: "kn_webhook_handler_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopeCluster, Unit: "s"},
}

// dataPlaneMetrics come from each revision pod's queue-proxy, scraped by the
// agent on its node.
var dataPlaneMetrics = []otelmetrics.MetricDefinition{
	{Name: "kn_serving_invocation_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, Unit: "s"},
}

var knativeMetrics = append(append([]otelmetrics.MetricDefinition{}, controlPlaneMetrics...), dataPlaneMetrics...)
