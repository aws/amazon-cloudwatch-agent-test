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

type DiskIOTestRunner struct {
}

var _ ITestRunner = (*DiskIOTestRunner)(nil)

func (m *DiskIOTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateDiskMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.getTestName(),
		TestResults: testResults,
	}
}

func (m *DiskIOTestRunner) getTestName() string {
	return "DiskIO"
}

func (m *DiskIOTestRunner) getAgentConfigFileName() string {
	return "diskio_config.json"
}

func (m *DiskIOTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *DiskIOTestRunner) getExtraCommands() []string {
	return []string{}
}

func (m *DiskIOTestRunner) getMeasuredMetrics() []string {
	return []string{
		"diskio_iops_in_progress", "diskio_io_time", "diskio_reads", "diskio_read_bytes", "diskio_read_time",
		"diskio_writes", "diskio_write_bytes", "diskio_write_time"}
}

func (m *DiskIOTestRunner) validateDiskMetric(metricName string) status.TestResult {
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
