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

var procStatSupportedMetricValues = map[string]struct{}{
	"procstat_cpu_time_system":  {},
	"procstat_cpu_time_user":    {},
	"procstat_cpu_usage":        {},
	"procstat_memory_data":      {},
	"procstat_memory_locked":    {},
	"procstat_memory_rss":       {},
	"procstat_memory_stack":     {},
	"procstat_memory_swap":      {},
	"procstat_memory_vms":       {},
	"procstat_pid":              {},
	"procstat_lookup_pid_count": {},
}

type ProcStatMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*ProcStatMetricValueFetcher)(nil)

func (f *ProcStatMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) ([]float64, error) {
	dims := f.getMetricSpecificDimensions()
	values, err := f.fetch(namespace, metricName, dims, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %v", metricName, err)
	}
	return values, err
}

func (f *ProcStatMetricValueFetcher) isApplicable(metricName string) bool {
	_, exists := procStatSupportedMetricValues[metricName]
	return exists
}

func (f *ProcStatMetricValueFetcher) getMetricSpecificDimensions() []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("exe"),
			Value: aws.String("cloudwatch-agent"),
		},
	}
}
