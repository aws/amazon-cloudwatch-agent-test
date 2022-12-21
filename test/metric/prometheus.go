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

type PrometheusMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*PrometheusMetricValueFetcher)(nil)

func (f *PrometheusMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := f.getMetricSpecificDimensions(metricName)
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %v: %v", metricName, err.Error())
	}
	return values, err
}

func (f *PrometheusMetricValueFetcher) isApplicable(metricName string) bool {
	prometheusSupportedMetric := f.getPluginSupportedMetric()
	_, exists := prometheusSupportedMetric[metricName]
	return exists
}

func (f *PrometheusMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// CWA currently only supports counters, gauges & summaries. We drop any untyped & histogram metrics and hence are "unsuppported" here.
	return map[string]struct{}{
		"prometheus_test_counter":       {},
		"prometheus_test_gauge":         {},
		"prometheus_test_summary_count": {},
		"prometheus_test_summary_sum":   {},
		"prometheus_test_summary":       {},
	}
}

func (f *PrometheusMetricValueFetcher) getMetricSpecificDimensions(metricName string) []types.Dimension {
	switch metricName {
	case "prometheus_test_counter":
		return []types.Dimension{
			{
				Name:  aws.String("prom_metric_type"),
				Value: aws.String("counter"),
			},
		}
	case "prometheus_test_gauge":
		return []types.Dimension{
			{
				Name:  aws.String("prom_metric_type"),
				Value: aws.String("gauge"),
			},
		}
	case "prometheus_test_summary_count":
		return []types.Dimension{
			{
				Name:  aws.String("prom_metric_type"),
				Value: aws.String("summary"),
			},
		}
	case "prometheus_test_summary_sum":
		return []types.Dimension{
			{
				Name:  aws.String("prom_metric_type"),
				Value: aws.String("summary"),
			},
		}
	case "prometheus_test_summary":
		return []types.Dimension{
			{
				Name:  aws.String("prom_metric_type"),
				Value: aws.String("summary"),
			},
			{
				Name:  aws.String("quantile"),
				Value: aws.String("0.5"),
			},
		}
	default:
		return []types.Dimension{}
	}
}
