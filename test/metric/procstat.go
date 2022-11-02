// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"log"
)

var procStatSupportedMetricValues = map[string]struct{}{
	"cpu_time_system": {},
	"cpu_time_user":   {},
	"cpu_usage":       {},
	"memory_data":     {},
	"memory_locked":   {},
	"memory_rss":      {},
	"memory_stack":    {},
	"memory_swap":     {},
	"memory_vms":      {},
	"pid":             {},
	"pid_count":       {},
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
	return []types.Dimension{}
}
