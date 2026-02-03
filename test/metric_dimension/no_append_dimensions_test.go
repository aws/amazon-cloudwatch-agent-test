// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
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
	specs := []DimensionSpec{HostDim(), ExactDim("cpu", "cpu-total")}
	return ValidateDimensionsPresent(&t.BaseTestRunner, "MetricAppendDimensionTest", metricName, specs)
}
