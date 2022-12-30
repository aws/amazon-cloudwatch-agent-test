// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type CPUTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*CPUTestRunner)(nil)

func (t *CPUTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName, bounds := range metricsToFetch {
		testResults = append(testResults, t.validateCpuMetric(metricName, bounds))
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *CPUTestRunner) getTestName() string {
	return "CPU"
}

func (t *CPUTestRunner) getAgentConfigFileName() string {
	return "cpu_config.json"
}

func (t *CPUTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *CPUTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"cpu_time_active":     nil,
		"cpu_time_guest":      nil,
		"cpu_time_guest_nice": nil,
		"cpu_time_idle":       nil,
		"cpu_time_iowait":     nil,
		"cpu_time_irq":        nil,
		"cpu_time_nice":       nil,
		"cpu_time_softirq":    nil,
		"cpu_time_steal":      nil,
		"cpu_time_system":     nil,
		"cpu_time_user":       nil,
		"cpu_usage_active": {
			Lower: 0.2,
			Upper: 0.4,
		},
		"cpu_usage_guest": {
			Lower: 0,
			Upper: 0,
		},
		"cpu_usage_guest_nice": {
			Lower: 0,
			Upper: 0,
		},
		"cpu_usage_idle": {
			Lower: 99,
			Upper: 100,
		},
		"cpu_usage_iowait": {
			Lower: 0.01,
			Upper: 0.05,
		},
		"cpu_usage_irq": {
			Lower: 0,
			Upper: 0,
		},
		"cpu_usage_nice": {
			Lower: 0,
			Upper: 0,
		},
		"cpu_usage_softirq": {
			Lower: 0,
			Upper: 0.005,
		},
		"cpu_usage_steal": {
			Lower: 0.05,
			Upper: 0.1,
		},
		"cpu_usage_system": {
			Lower: 0.05,
			Upper: 0.2,
		},
		"cpu_usage_user": {
			Lower: 0.02,
			Upper: 0.07,
		},
	}
}

func (t *CPUTestRunner) validateCpuMetric(metricName string, bounds *metric.Bounds) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := t.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	testResult.Values = values

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	if bounds != nil && !IsMetricWithinBounds(values, *bounds) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
