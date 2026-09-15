//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron_dra

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Neuron metric definitions (same source names as the device-plugin neuron package;
// OTel preserves raw neuron-monitor names regardless of allocation mechanism).
// neuroncore_utilization_ratio is the per-core metric the correlation tests use.
var neuronMetrics = []otelmetrics.MetricDefinition{
	{Name: "neuroncore_utilization_ratio", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "1"},
	{Name: "neuroncore_memory_usage_model_shared_scratchpad", MetricType: "gauge", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "neuron_runtime_memory_used_bytes", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"memory_location"}, Unit: "By"},
	{Name: "execution_latency_seconds", MetricType: "gauge", Scope: otelmetrics.ScopePod, ExpectedLabels: []string{"percentile"}, Unit: "s"},
}
