// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"time"
)

type ProcStatTestRunner struct {
}

var _ ITestRunner = (*ProcStatTestRunner)(nil)

func (m *ProcStatTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateMemMetric(name)
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

func (m *ProcStatTestRunner) getMeasuredMetrics() []string {
	return []string{
		"cpu_time_system", "cpu_time_user", "cpu_usage", "memory_data", "memory_locked",
		"memory_rss", "memory_stack", "memory_swap", "memory_vms", "pid", "pid_count"}
}

func (m *ProcStatTestRunner) validateMemMetric(metricName string) status.TestResult {
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
