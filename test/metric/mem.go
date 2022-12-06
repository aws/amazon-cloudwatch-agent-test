// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type MemMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*MemMetricValueFetcher)(nil)

func (f *MemMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dims := f.getMetricSpecificDimensions()
	dims = append(dims, f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dims, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *MemMetricValueFetcher) isApplicable(metricName string) bool {
	memSupportedMetric := f.getPluginSupportedMetric()
	_, exists := memSupportedMetric[metricName]
	return exists
}

func (f *MemMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// Memory Supported Metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L14
	return map[string]struct{}{
		"mem_active":            {},
		"mem_available":         {},
		"mem_available_percent": {},
		"mem_buffered":          {},
		"mem_cached":            {},
		"mem_free":              {},
		"mem_inactive":          {},
		"mem_total":             {},
		"mem_used":              {},
		"mem_used_percent":      {},
	}
}

func (f *MemMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{}
}
