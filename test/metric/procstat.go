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

type ProcStatMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*ProcStatMetricValueFetcher)(nil)

func (f *ProcStatMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *ProcStatMetricValueFetcher) isApplicable(metricName string) bool {
	procStatSupportedMetric := f.getPluginSupportedMetric()
	_, exists := procStatSupportedMetric[metricName]
	return exists
}

func (f *ProcStatMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// Procstat Supported Metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L19-L23
	return map[string]struct{}{
		"procstat_cpu_time_system": {},
		"procstat_cpu_time_user":   {},
		"procstat_cpu_usage":       {},
		"procstat_memory_data":     {},
		"procstat_memory_locked":   {},
		"procstat_memory_rss":      {},
		"procstat_memory_stack":    {},
		"procstat_memory_swap":     {},
		"procstat_memory_vms":      {},
	}
}

func (f *ProcStatMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("exe"),
			Value: aws.String("cloudwatch-agent"),
		},
		{
			Name:  aws.String("process_name"),
			Value: aws.String("amazon-cloudwatch-agent"),
		},
	}
}
