// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type DiskIOTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*DiskIOTestRunner)(nil)

func (m *DiskIOTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateDiskMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *DiskIOTestRunner) GetTestName() string {
	return "DiskIO"
}

func (m *DiskIOTestRunner) GetAgentConfigFileName() string {
	return "diskio_config.json"
}

func (m *DiskIOTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (m *DiskIOTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"diskio_iops_in_progress", "diskio_io_time", "diskio_reads", "diskio_read_bytes", "diskio_read_time",
		"diskio_writes", "diskio_write_bytes", "diskio_write_time"}
}

func (m *DiskIOTestRunner) validateDiskMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher := metric.NewMetricValueFetcher{Env: &environment.MetaData{}, ExpectedDimensionNames: []string{"InstanceId"}}

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
