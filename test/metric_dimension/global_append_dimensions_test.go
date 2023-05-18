// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
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
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	expDims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "ImageId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceType",
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
	values, err := fetcher.Fetch("MetricGlobalAppendDimensionTest", metricName, expDims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	// this is making sure once dimensions in "append_dimensions" are tagged, the agent does drop the
	// host dimension. We should not see the same metrics with host dimension anymore
	dropDims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
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

	values, err = fetcher.Fetch("MetricGlobalAppendDimensionTest", metricName, dropDims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil || len(values) != 0 {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
