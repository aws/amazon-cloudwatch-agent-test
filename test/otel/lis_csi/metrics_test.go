//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package lis_csi

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// lisCsiVolumeMetrics are per-volume metrics that carry instance_id and volume_id.
var lisCsiVolumeMetrics = []otelmetrics.MetricDefinition{
	{Name: "aws_ec2_instance_store_csi_read_ops_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_write_ops_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_read_bytes_total", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "aws_ec2_instance_store_csi_write_bytes_total", MetricType: "counter", Scope: otelmetrics.ScopePod, Unit: "By"},
	{Name: "aws_ec2_instance_store_csi_read_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_write_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_ec2_exceeded_iops_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_ec2_exceeded_tp_seconds_total", MetricType: "counter", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_volume_queue_length", MetricType: "gauge", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_read_io_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod},
	{Name: "aws_ec2_instance_store_csi_write_io_latency_seconds", MetricType: "histogram", Scope: otelmetrics.ScopePod},
}

// lisCsiCollectorMetrics are collector-internal metrics that do not carry volume_id/instance_id.
var lisCsiCollectorMetrics = []otelmetrics.MetricDefinition{
	{Name: "aws_ec2_instance_store_csi_nvme_collector_errors_total", MetricType: "counter", Scope: otelmetrics.ScopeNode},
	{Name: "aws_ec2_instance_store_csi_nvme_collector_scrapes_total", MetricType: "counter", Scope: otelmetrics.ScopeNode},
}

func lisCsiMetricNames() []string {
	all := append(lisCsiVolumeMetrics, lisCsiCollectorMetrics...)
	names := make([]string, len(all))
	for i, d := range all {
		names[i] = d.Name
	}
	return names
}

func lisCsiVolumeMetricNames() []string {
	names := make([]string, len(lisCsiVolumeMetrics))
	for i, d := range lisCsiVolumeMetrics {
		names[i] = d.Name
	}
	return names
}
