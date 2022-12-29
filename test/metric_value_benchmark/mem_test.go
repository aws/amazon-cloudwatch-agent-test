// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type MemTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*MemTestRunner)(nil)

func (m *MemTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateMemMetric(name))
	}

	return status.TestGroupResult{
		Name:        m.getTestName(),
		TestResults: testResults,
	}
}

func (m *MemTestRunner) getTestName() string {
	return "Mem"
}

func (m *MemTestRunner) getAgentConfigFileName() string {
	return "mem_config.json"
}

func (m *MemTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (m *MemTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"mem_active":            nil,
		"mem_available":         nil,
		"mem_available_percent": nil,
		"mem_buffered":          nil,
		"mem_cached":            nil,
		"mem_free":              nil,
		"mem_inactive":          nil,
		"mem_total":             nil,
		"mem_used":              nil,
		"mem_used_percent":      nil,
	}
}

func (m *MemTestRunner) validateMemMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := m.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
