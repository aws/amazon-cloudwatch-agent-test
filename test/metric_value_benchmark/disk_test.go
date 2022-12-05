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

type DiskTestRunner struct{
	BaseTestRunner
}

var _ ITestRunner = (*DiskTestRunner)(nil)

func (t *DiskTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateDiskMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *DiskTestRunner) getTestName() string {
	return "Disk"
}

func (t *DiskTestRunner) getAgentConfigFileName() string {
	return "disk_config.json"
}
func (t *DiskTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *DiskTestRunner) getMeasuredMetrics() []string {
	return []string{
		"disk_free",
		"disk_inodes_free",
		"disk_inodes_total",
		"disk_inodes_used",
		"disk_total",
		"disk_used",
		"disk_used_percent",
	}
}

func (t *DiskTestRunner) validateDiskMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := t.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}
	
	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
