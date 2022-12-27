// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows
// +build !windows

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type EMFMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*EMFMetricValueFetcher)(nil)

func (f *EMFMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(metricName), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *EMFMetricValueFetcher) isApplicable(metricName string) bool {
	emfSupportedMetric := f.getPluginSupportedMetric()
	_, exists := emfSupportedMetric[metricName]
	return exists
}

func (f *EMFMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	return map[string]struct{}{
		"EMFCounter": {},
	}
}
func (f *EMFMetricValueFetcher) getMetricSpecificDimensions(string) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("Type"),
			Value: aws.String("Counter"),
		},
	}
}
