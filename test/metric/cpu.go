// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux
// +build linux

package metric

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type CPUMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*CPUMetricValueFetcher)(nil)

func (f *CPUMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *CPUMetricValueFetcher) isApplicable(metricName string) bool {
	cpuSupportedMetric := f.getPluginSupportedMetric()
	_, exists := cpuSupportedMetric[metricName]
	return exists
}

func (f *CPUMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// CPU supported metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L9-L10
	return map[string]struct{}{
		"cpu_time_active":      {},
		"cpu_time_guest":       {},
		"cpu_time_guest_nice":  {},
		"cpu_time_idle":        {},
		"cpu_time_iowait":      {},
		"cpu_time_irq":         {},
		"cpu_time_nice":        {},
		"cpu_time_softirq":     {},
		"cpu_time_steal":       {},
		"cpu_time_system":      {},
		"cpu_time_user":        {},
		"cpu_usage_active":     {},
		"cpu_usage_guest":      {},
		"cpu_usage_guest_nice": {},
		"cpu_usage_idle":       {},
		"cpu_usage_iowait":     {},
		"cpu_usage_irq":        {},
		"cpu_usage_nice":       {},
		"cpu_usage_softirq":    {},
		"cpu_usage_steal":      {},
		"cpu_usage_system":     {},
		"cpu_usage_user":       {},
	}
}

func (f *CPUMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("cpu"),
			Value: aws.String("cpu-total"),
		},
	}
}
