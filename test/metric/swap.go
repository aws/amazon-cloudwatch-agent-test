
// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type SwapMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*SwapMetricValueFetcher)(nil)

func (f *SwapMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions()
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for $s: $v", metricName, err)
	}
	return values, err
}

func (f *SwapMetricValueFetcher) isApplicable(metricName string) bool {
	swapSupportedMetric := f.getPluginSupportedMetric()
	_, exists := swapSupportedMetric[metricName]
	return exists
}

// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L35
func (f *SwapMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	return map[string]struct{}{
		"swap_free":         {},
		"swap_used":  {},
		"swap_used_percent": {},
	}
}

func (f *SwapMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{}
}