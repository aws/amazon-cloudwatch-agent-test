// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type NetStatMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*NetStatMetricValueFetcher)(nil)

func (f *NetStatMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(metricName), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L38
func (f *NetStatMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	return map[string]struct{}{
		"netstat_tcp_close":       {},
		"netstat_tcp_close_wait":  {},
		"netstat_tcp_closing":     {},
		"netstat_tcp_established": {},
		"netstat_tcp_fin_wait1":   {},
		"netstat_tcp_fin_wait2":   {},
		"netstat_tcp_last_ack":    {},
		"netstat_tcp_listen":      {},
		"netstat_tcp_none":        {},
		"netstat_tcp_syn_sent":    {},
		"netstat_tcp_syn_recv":    {},
		"netstat_tcp_time_wait":   {},
		"netstat_udp_socket":      {},
	}
}

func (f *NetStatMetricValueFetcher) getMetricSpecificDimensions(string) []types.Dimension {
	return []types.Dimension{}
}
