// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"

	"time"
)

type ProcStatTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*ProcStatTestRunner)(nil)

func (m *ProcStatTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateProcStatMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *ProcStatTestRunner) GetTestName() string {
	return "ProcStat"
}

func (m *ProcStatTestRunner) GetAgentConfigFileName() string {
	return "procstat_config.json"
}

func (m *ProcStatTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (t *ProcStatTestRunner) SetupAfterAgentRun() error {
	return nil
}

func (m *ProcStatTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"procstat_cpu_time_system", "procstat_cpu_time_user", "procstat_cpu_usage", "procstat_memory_data", "procstat_memory_locked",
		"procstat_memory_rss", "procstat_memory_stack", "procstat_memory_swap", "procstat_memory_vms"}
}

func (m *ProcStatTestRunner) validateProcStatMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher := metric.MetricValueFetcher{Env: &environment.MetaData{}, ExpectedDimensionNames: []string{"InstanceId"}}

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
