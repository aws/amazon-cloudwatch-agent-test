// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type DiskTestRunner struct{}

var _ ITestRunner = (*DiskTestRunner)(nil)

func (t *DiskTestRunner) struct() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = validateDiskMetric
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
	return []string {
		"disk_free", "disk_inodes_free", "disk_inodes_total", "disk_inodes_used", "disk_total", "disk_used", "disk_used_percent"
	}
}

func validateDiskMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name: metricName,
		Status: status.FAILED,
	}

	fetcher, err := metric.GetMetricFetcher(metricName)
	if (err != nil) { return testResult }

	values := err = fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil { return testResult }

	testResult.Status = status.SUCCESSFUL
	return testResult
}
