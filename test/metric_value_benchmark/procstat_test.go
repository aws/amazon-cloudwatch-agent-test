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

type ProcStatTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*ProcStatTestRunner)(nil)

func (m *ProcStatTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateProcStatMetric(name))
	}

	return status.TestGroupResult{
		Name:        m.getTestName(),
		TestResults: testResults,
	}
}

func (m *ProcStatTestRunner) getTestName() string {
	return "ProcStat"
}

func (m *ProcStatTestRunner) getAgentConfigFileName() string {
	return "procstat_config.json"
}

func (m *ProcStatTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (m *ProcStatTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"procstat_cpu_time_system": nil,
		"procstat_cpu_time_user":   nil,
		"procstat_cpu_usage":       nil,
		"procstat_memory_data":     nil,
		"procstat_memory_locked":   nil,
		"procstat_memory_rss":      nil,
		"procstat_memory_stack":    nil,
		"procstat_memory_swap":     nil,
		"procstat_memory_vms":      nil,
	}
}

func (m *ProcStatTestRunner) validateProcStatMetric(metricName string) status.TestResult {
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
