//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package vllm

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// vllmMetrics are the vLLM engine and API-server metrics expected from every
// vLLM server the chart finds. The chart keeps vllm:* and http_* and drops
// python_*, process_*, *_created and summaries.
var vllmMetrics = []otelmetrics.MetricDefinition{
	// Scheduler state
	{Name: "vllm:num_requests_running", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "vllm:num_requests_waiting", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "vllm:kv_cache_usage_perc", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},

	// Tokens and requests
	{Name: "vllm:prompt_tokens_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "vllm:generation_tokens_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}},
	{Name: "vllm:request_success_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name", "finished_reason"}},

	// Latency
	{Name: "vllm:e2e_request_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},
	{Name: "vllm:time_to_first_token_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},
	{Name: "vllm:inter_token_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},
	{Name: "vllm:request_queue_time_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"model_name"}, Unit: "s"},

	// API server (prometheus-fastapi-instrumentator)
	{Name: "http_requests_total", MetricType: "counter", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"handler", "method", "status"}},
	{Name: "http_request_duration_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"handler", "method"}, Unit: "s"},
}
