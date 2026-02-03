// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type GlobalAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*GlobalAppendDimensionsTestRunner)(nil)

func (t *GlobalAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateGlobalAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *GlobalAppendDimensionsTestRunner) GetTestName() string {
	return "GlobalAppendDimension"
}

func (t *GlobalAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "global_append_dimension.json"
}

func (t *GlobalAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active"}
}

func (t *GlobalAppendDimensionsTestRunner) validateGlobalAppendDimensionMetric(metricName string) status.TestResult {
	ns := "MetricGlobalAppendDimensionTest"

	presentSpecs := []DimensionSpec{
		{Key: "ImageId"},
		{Key: "InstanceId"},
		{Key: "InstanceType"},
		ExactDim("cpu", "cpu-total"),
	}
	droppedSpecs := []DimensionSpec{HostDim(), ExactDim("cpu", "cpu-total")}

	return ValidateGlobalAppendDimensions(&t.BaseTestRunner, ns, metricName, presentSpecs, droppedSpecs)
}
