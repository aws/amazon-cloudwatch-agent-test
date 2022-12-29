// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type SwapTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*SwapTestRunner)(nil)

func (t *SwapTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validateSwapMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
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

func (t *SwapTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"swap_free":         nil,
		"swap_used":         nil,
		"swap_used_percent": nil,
	}
}

func (t *SwapTestRunner) validateSwapMetric(metricName string) status.TestResult {
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
