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
	BaseTestRunner
}

var _ ITestRunner = (*DiskIOTestRunner)(nil)

func (m *DiskIOTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateDiskMetric(name))
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

func (m *DiskIOTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"diskio_iops_in_progress": nil,
		"diskio_io_time":          nil,
		"diskio_reads":            nil,
		"diskio_read_bytes":       nil,
		"diskio_read_time":        nil,
		"diskio_writes":           nil,
		"diskio_write_bytes":      nil,
		"diskio_write_time":       nil,
	}
}

func (m *DiskIOTestRunner) validateDiskMetric(metricName string) status.TestResult {
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
