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

type SwapTestRunner struct{}

var _ ITestRunner = (*SwapTestRunner)(nil)

func (t *SwapTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = validateSwapmetric(metricName)
	}

	return status.TestGroupResult{
		Name: t.getTestName(),
		TestResults: testResults,
	}
}


func (t *SwapTestRunner) getTestName() string {
	return "Swap"
}

func (t *SwapTestRunner) getAgentConfigFileName() string {
	return "swap_config.json"
}
func (t *SwapTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *SwapTestRunner) getMeasuredMetrics() []string {
	return []string {
		"swap_free",
		"swap_used",
		"swap_used_percent",
	}
}

func validateSwapmetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name: metricName,
		Status: status.FAILED,
	}

	fetcher, err := metric.GetMetricFetcher(metricName)
	if err != nil { return testResult }

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil { return testResult }

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) { return testResult }

	testResult.Status = status.SUCCESSFUL
	return testResult
}