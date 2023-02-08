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

// validate checks that a metric exists with each of the expected dimension sets.
func (t *AggregationDimensionsTestRunner) validate(metricName string) status.TestResult {
	r := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	// Expect to find the metric with each of the following lists of dimensions.
	// Except the last one - "InstanceType".
	aggregations := [][]string{
		{},
		{"InstanceId"},
		{"InstanceId", "InstanceType"},
		{"InstanceType"},
	}
	for i, aggregation := range aggregations {
		instructions := []dimension.Instruction{}
		for _, d := range aggregation {
			instruction := dimension.Instruction{
				Key: d,
				Value: dimension.UnknownDimensionValue(),
			}
			instructions = append(instructions, instruction)
		}
		dd, _ := t.DimensionFactory.GetDimensions(instructions)
		f := metric.MetricValueFetcher{}
		values, err := f.Fetch("TestAggregationDimensions", metricName, dd,
			metric.AVERAGE, test_runner.HighResolutionStatPeriod)
		// Do not expect the last aggregation of just "InstanceType".
		if i == len(aggregations)-1 && len(values) > 0 {
			log.Printf("Expected no metrics with these dimensions - %v", aggregation)
			return r
		} else {
			if err != nil || !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
				return r
			}
		}
	}
	r.Status = status.SUCCESSFUL
	return r
}
