// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_append_dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"

	"log"
	"time"
)

type NoAppendDimensionTestRunner struct {
	Base test_runner.BaseTestRunner
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

func (t *NoAppendDimensionTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *NoAppendDimensionTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active"}
}

func (t *NoAppendDimensionTestRunner) validateNoAppendDimensionMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.Base.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:  "host",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:  "cpu",
			Value: dimension.ExpectedDimensionValue{aws.String("cpu-total")},
		},
	})

	if (len(failed) > 0) {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("MetricAppendDimensionTest", metricName, dims, metric.AVERAGE)
	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}
