// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric

import (
	"log"
)

type ProcessesMetricValueFetcher struct {
	baseMetricValueFetcher
}

var _ MetricValueFetcher = (*ProcessesMetricValueFetcher)(nil)

func (f *ProcessesMetricValueFetcher) Fetch(namespace, metricName string, stat Statistics) (MetricValues, error) {
	dimensions := append(f.getMetricSpecificDimensions(metricName), f.getInstanceIdDimension())
	values, err := f.fetch(namespace, metricName, dimensions, stat)
	if err != nil {
		log.Printf("Error while fetching metric value for %s: %s", metricName, err.Error())
	}
	return values, err
}

func (f *ProcessesMetricValueFetcher) isApplicable(metricName string) bool {
	processesSupportedMetric := f.getPluginSupportedMetric()
	_, exists := processesSupportedMetric[metricName]
	return exists
}

func (f *ProcessesMetricValueFetcher) getPluginSupportedMetric() map[string]struct{} {
	// Processes Supported Metrics
	// https://github.com/aws/amazon-cloudwatch-agent/blob/6451e8b913bcf9892f2cead08e335c913c690e6d/translator/translate/metrics/config/registered_metrics.go#L17
	return map[string]struct{}{
		"processes_blocked":       {},
		"processes_dead":          {},
		"processes_idle":          {},
		"processes_paging":        {},
		"processes_running":       {},
		"processes_sleeping":      {},
		"processes_stopped":       {},
		"processes_total":         {},
		"processes_total_threads": {},
		"processes_wait":          {},
		"processes_zombies":       {},
	}
}
