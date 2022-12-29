// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type NetMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*NetMetricValueFetcher)(nil)

func (f *NetMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(metricName), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *NetMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// Net Supported Metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L15
	return map[string]struct{}{
		"net_bytes_sent":   {},
		"net_bytes_recv":   {},
		"net_drop_in":      {},
		"net_drop_out":     {},
		"net_err_in":       {},
		"net_err_out":      {},
		"net_packets_sent": {},
		"net_packets_recv": {},
	}
}

func (f *NetMetricValueFetcher) getMetricSpecificDimensions(string) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("interface"),
			Value: aws.String("docker0"),
		},
	}
}
