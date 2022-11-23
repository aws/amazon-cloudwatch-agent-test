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
}

var _ ITestRunner = (*ProcStatTestRunner)(nil)

func (m *ProcStatTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateProcStatMetric(name)
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

func (t *ProcStatTestRunner) setupBeforeAgentRun() error {
	return nil
}

func (m *ProcStatTestRunner) getMeasuredMetrics() []string {
	return []string{
		"procstat_cpu_time_system", "procstat_cpu_time_user", "procstat_cpu_usage", "procstat_memory_data", "procstat_memory_locked",
		"procstat_memory_rss", "procstat_memory_stack", "procstat_memory_swap", "procstat_memory_vms"}
}

func (m *ProcStatTestRunner) validateProcStatMetric(metricName string) status.TestResult {
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
