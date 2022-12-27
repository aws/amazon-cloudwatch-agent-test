// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package _default

import (
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type PrometheusMetricDimension struct {
}

var _ MetricDefaultDimensionFactory = (*PrometheusMetricDimension)(nil)

func (f *PrometheusMetricDimension) isApplicable(metricName string) bool {
	prometheusSupportedMetric := f.getPluginSupportedMetric()
	_, exists := prometheusSupportedMetric[metricName]
	return exists
}

func (f *PrometheusMetricDimension) getPluginSupportedMetric() map[string]struct{} {
	// CWA currently only supports counters, gauges & summaries. We drop any untyped & histogram metrics and hence are "unsuppported" here.
	return map[string]struct{}{
		"prometheus_test_counter":       {},
		"prometheus_test_gauge":         {},
		"prometheus_test_summary_count": {},
		"prometheus_test_summary_sum":   {},
		"prometheus_test_summary":       {},
	}
}

func (f *PrometheusMetricDimension) getMetricSpecificDimensions(env environment.MetaData) []types.Dimension {
	return []types.Dimension{}
}
