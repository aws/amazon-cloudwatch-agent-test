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
}

var _ ITestRunner = (*ProcessesTestRunner)(nil)

func (m *ProcessesTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateProcessesMetric(name)
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

func (m *ProcessesTestRunner) getMeasuredMetrics() []string {
	return []string{
		"processes_blocked", "processes_dead", "processes_idle", "processes_paging", "processes_running", "processes_sleeping", "processes_stopped",
		"processes_total", "processes_total_threads", "processes_wait", "processes_zombies"}
}

func (m *ProcessesTestRunner) validateProcessesMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := metric.GetMetricFetcher(metricName)
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
