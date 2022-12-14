// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package _default

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type MemMetricDimension struct {
}

var _ MetricDefaultDimensionFactory = (*MemMetricDimension)(nil)

func (f *MemMetricDimension) isApplicable(metricName string) bool {
	memSupportedMetric := f.getPluginSupportedMetric()
	_, exists := memSupportedMetric[metricName]
	return exists
}

func (f *MemMetricDimension) getPluginSupportedMetric() map[string]struct{} {
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

func (f *MemMetricDimension) getMetricSpecificDimensions(env environment.MetaData) []types.Dimension {
	return []types.Dimension{}
}
