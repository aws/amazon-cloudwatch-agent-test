//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package multi_efa

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

var efaMetrics = []otelmetrics.MetricDefinition{
	{Name: "efa_rx_bytes", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "efa_tx_bytes", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "efa_rx_dropped", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "efa_rdma_read_bytes", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "By"},
}

var efaMetricNamesList = metricNames(efaMetrics)

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}
