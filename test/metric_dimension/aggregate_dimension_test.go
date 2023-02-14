// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type OneAggregateDimensionTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*OneAggregateDimensionTestRunner)(nil)

func (t *OneAggregateDimensionTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateNoAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *OneAggregateDimensionTestRunner) GetTestName() string {
	return "OneAggregatedDimension"
}

func (t *OneAggregateDimensionTestRunner) GetAgentConfigFileName() string {
	return "one_aggregate_dimension.json"
}

func (t *OneAggregateDimensionTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active", "cpu_time_guest"}
}

func (t *OneAggregateDimensionTestRunner) validateNoAppendDimensionMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("MetricAggregateDimensionTest", metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL

	return testResult
}
