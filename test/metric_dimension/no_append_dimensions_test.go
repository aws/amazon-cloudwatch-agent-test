// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type NoAppendDimensionTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*NoAppendDimensionTestRunner)(nil)

func (t *NoAppendDimensionTestRunner) Validate() status.TestGroupResult {
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

func (t *NoAppendDimensionTestRunner) GetTestName() string {
	return "NoAppendDimension"
}

func (t *NoAppendDimensionTestRunner) GetAgentConfigFileName() string {
	return "no_append_dimension.json"
}

func (t *NoAppendDimensionTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active"}
}

func (t *NoAppendDimensionTestRunner) validateNoAppendDimensionMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "host",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("MetricAppendDimensionTest", metricName, dims, metric.AVERAGE, common.HighResolutionStatPeriod)
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
