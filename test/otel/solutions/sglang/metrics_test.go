//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package sglang

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// sglangMetrics are SGLang engine metrics expected from every SGLang server the
// chart finds, once it has served requests. The chart keeps sglang:* and drops
// *_created and summaries. Families SGLang creates only for streaming requests
// (inter-token latency, cached tokens) are not listed.
var sglangMetrics = []otelmetrics.MetricDefinition{
	// Scheduler state
	{Name: "sglang:num_running_reqs", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "engine_type"}},
	{Name: "sglang:num_queue_reqs", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "engine_type"}},
	{Name: "sglang:token_usage", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "engine_type"}},
	{Name: "sglang:gen_throughput", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "engine_type"}},
	{Name: "sglang:cache_hit_rate", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "engine_type"}},

	// Tokens and requests
	{Name: "sglang:prompt_tokens_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "sglang:generation_tokens_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "sglang:num_requests_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},

	// Latency
	{Name: "sglang:e2e_request_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},
	{Name: "sglang:time_to_first_token_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},
	{Name: "sglang:queue_time_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},

	// API server
	{Name: "sglang:http_requests_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"endpoint", "method"}},
}
