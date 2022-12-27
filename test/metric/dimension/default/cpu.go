// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package _default

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type CPUMetricDimension struct {
}

var _ MetricDefaultDimensionFactory = (*CPUMetricDimension)(nil)

func (f *CPUMetricDimension) isApplicable(metricName string) bool {
	cpuSupportedMetric := f.getPluginSupportedMetric()
	_, exists := cpuSupportedMetric[metricName]
	return exists
}

func (f *CPUMetricDimension) getPluginSupportedMetric() map[string]struct{} {
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

func (f *CPUMetricDimension) getMetricSpecificDimensions(env environment.MetaData) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("cpu"),
			Value: aws.String("cpu-total"),
		},
	}
}
