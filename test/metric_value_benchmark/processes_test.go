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

type ProcessesTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*ProcessesTestRunner)(nil)

func (m *ProcessesTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateProcessesMetric(name))
	}

	return status.TestGroupResult{
		Name:        m.getTestName(),
		TestResults: testResults,
	}
}

func (m *ProcessesTestRunner) getTestName() string {
	return "Processes"
}

func (m *ProcessesTestRunner) getAgentConfigFileName() string {
	return "processes_config.json"
}

func (m *ProcessesTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (m *ProcessesTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"processes_blocked":       nil,
		"processes_dead":          nil,
		"processes_idle":          nil,
		"processes_paging":        nil,
		"processes_running":       nil,
		"processes_sleeping":      nil,
		"processes_stopped":       nil,
		"processes_total":         nil,
		"processes_total_threads": nil,
		"processes_zombies":       nil,
	}
}

func (m *ProcessesTestRunner) validateProcessesMetric(metricName string) status.TestResult {
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
