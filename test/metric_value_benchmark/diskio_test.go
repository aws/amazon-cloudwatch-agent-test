// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type DiskIOTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*DiskIOTestRunner)(nil)

func (m *DiskIOTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateDiskMetric(name))
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

func (m *DiskIOTestRunner) GetMeasuredMetrics() map[string]*metric.Bounds {
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

	dims, failed := m.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "name",
			Value: dimension.ExpectedDimensionValue{aws.String("nvme0n1")},
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
