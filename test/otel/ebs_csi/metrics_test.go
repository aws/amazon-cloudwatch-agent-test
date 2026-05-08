//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ebs_csi

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

var ebsCsiMetrics = []otelmetrics.MetricDefinition{
	{Name: "aws_ebs_csi_read_ops_total", MetricType: "counter"},
	{Name: "aws_ebs_csi_write_ops_total", MetricType: "counter"},
	{Name: "aws_ebs_csi_read_bytes_total", MetricType: "counter", Unit: "By"},
	{Name: "aws_ebs_csi_write_bytes_total", MetricType: "counter", Unit: "By"},
	{Name: "aws_ebs_csi_volume_queue_length", MetricType: "gauge"},
}

func ebsCsiMetricNames() []string {
	names := make([]string, len(ebsCsiMetrics))
	for i, d := range ebsCsiMetrics {
		names[i] = d.Name
	}
	return names
}
