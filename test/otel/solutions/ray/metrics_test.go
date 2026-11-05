//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ray

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// nodeMetrics are Ray core metrics every Ray pod serves on its metrics port.
var nodeMetrics = []otelmetrics.MetricDefinition{
	{Name: "ray_node_cpu_utilization", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"SessionName"}},
	{Name: "ray_node_mem_used", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"SessionName"}},
	{Name: "ray_object_store_memory", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"SessionName"}},
}

// serveMetrics are Ray Serve metrics, served by whichever Ray pod runs the
// proxy or the replica.
var serveMetrics = []otelmetrics.MetricDefinition{
	{Name: "ray_serve_num_http_requests_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"route"}},
	{Name: "ray_serve_http_request_latency_ms", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"route"}, Unit: "ms"},
	{Name: "ray_serve_deployment_request_counter_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"deployment"}},
	{Name: "ray_serve_deployment_processing_latency_ms", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"deployment"}, Unit: "ms"},
	{Name: "ray_serve_deployment_replica_healthy", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"deployment"}},
}
