// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type AggregationDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*AggregationDimensionsTestRunner)(nil)

func (t *AggregationDimensionsTestRunner) Validate() status.TestGroupResult {
	mm := t.GetMeasuredMetrics()
	r := make([]status.TestResult, len(mm))
	for i, m := range mm {
		r[i] = t.validate(m)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: r,
	}
}

func (t *AggregationDimensionsTestRunner) GetTestName() string {
	return "AggregationDimensionsTestRunner"
}

func (t *AggregationDimensionsTestRunner) GetAgentConfigFileName() string {
	return "aggregation_dimensions.json"
}

func (t *AggregationDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"mem_used_percent"}
}

// validate checks that a metric exists with each of the expected dimension lists.
func (t *AggregationDimensionsTestRunner) validate(metricName string) status.TestResult {
	r := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	dd, errs := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceType",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})
	if len(errs) > 0 {
		return r
	}
	f := metric.MetricValueFetcher{}
	values, err := f.Fetch("TestAggregationDimensions", metricName, dd,
		metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	log.Printf("metric values are %v", values)
	if err != nil {
		return r
	}
	if isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		r.Status = status.SUCCESSFUL
	}
	return r
}
