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

type CollectDMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*CollectDMetricValueFetcher)(nil)

func (f *CollectDMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions()
	dimensions = append(dimensions, f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %v: %v", metricName, err.Error())
	}
	return values, err
}

func (f *CollectDMetricValueFetcher) isApplicable(metricName string) bool {
	collectdSupportedMetric := f.getPluginSupportedMetric()
	_, exists := collectdSupportedMetric[metricName]
	return exists
}

func (f *CollectDMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// Use CPU plugins with CollectD will only returns collectd_cpu_value; however, the type_instance dimension will specify
	// the real metric
	return map[string]struct{}{
		"collectd_cpu_value": {},
	}
}
func (f *CollectDMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("type_instance"),
			Value: aws.String("user"),
		},
		{
			Name:  aws.String("instance"),
			Value: aws.String("0"),
		},
		{
			Name:  aws.String("type"),
			Value: aws.String("percent"),
		},
	}
}
