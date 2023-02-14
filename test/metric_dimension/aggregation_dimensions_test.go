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
	"github.com/aws/aws-sdk-go-v2/aws"
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
	return []string{"mem_used_percent", "cpu_usage_user"}
}

// getExpectedDimensions returns a list of expected dimensions for the given metric name.
// This is based on the JSON configuration.
func getExpectedDimensions(metricName string) [][]string {
	switch metricName {
	case "mem_used_percent":
		return [][]string{
			{"foo", "bar", "baz", "InstanceId", "InstanceType"},
			{},
			{"InstanceId"},
			{"InstanceId", "InstanceType"},
			{"foo", "bar", "InstanceType"},
		}
	case "cpu_usage_user":
		return [][]string{
			{"InstanceId", "InstanceType"},
			{},
			{"InstanceId"},
		}
	}
	return nil
}

func getUnexpectedDimensions(metricName string) [][]string {
	switch metricName {
	case "mem_used_percent":
		return [][]string{
			{"foo"},
			{"foo", "bar"},
			{"InstanceType"},
		}
	case "cpu_usage_user":
		return [][]string{
			{"foo"},
			{"foo", "bar"},
			{"InstanceType"},
			{"foo", "bar", "baz", "InstanceId", "InstanceType"},
		}
	}
	return nil
}

func getDimensionValue(dimensionKey string) dimension.ExpectedDimensionValue {
	switch dimensionKey {
	case "foo":
		return dimension.ExpectedDimensionValue{aws.String("fooval")}
	case "bar":
		return dimension.ExpectedDimensionValue{aws.String("barval")}
	case "baz":
		return dimension.ExpectedDimensionValue{aws.String("bazval")}
	default:
		return dimension.UnknownDimensionValue()
	}
}

// validate checks that a metric exists with each of the expected dimension sets.
func (t *AggregationDimensionsTestRunner) validate(metricName string) status.TestResult {
	r := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	// Validate the metric name with some expected dimension sets.
	aggregations := getExpectedDimensions(metricName)
	for _, aggregation := range aggregations {
		instructions := []dimension.Instruction{}
		for _, k := range aggregation {
			v := getDimensionValue(k)
			i := dimension.Instruction{Key: k, Value: v}
			instructions = append(instructions, i)
		}
		dd, _ := t.DimensionFactory.GetDimensions(instructions)
		f := metric.MetricValueFetcher{}
		values, err := f.Fetch("TestAggregationDimensions", metricName, dd,
			metric.AVERAGE, test_runner.HighResolutionStatPeriod)
		// Expect values for the metric with the current dimension list.
		if err != nil || !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
			return r
		}
	}

	// Validate the metric with some dimensions DO NOT EXIST.
	aggregations = getUnexpectedDimensions(metricName)
	for _, aggregation := range aggregations {
		instructions := []dimension.Instruction{}
		for _, k := range aggregation {
			v := getDimensionValue(k)
			i := dimension.Instruction{Key: k, Value: v}
			instructions = append(instructions, i)
		}
		dd, _ := t.DimensionFactory.GetDimensions(instructions)
		f := metric.MetricValueFetcher{}
		values, _ := f.Fetch("TestAggregationDimensions", metricName, dd,
			metric.AVERAGE, test_runner.HighResolutionStatPeriod)
		// Expect no values.
		if len(values) > 0 {
			log.Printf("Expected no metrics with these dimensions - %v", aggregation)
			return r
		}
	}
	r.Status = status.SUCCESSFUL
	return r
}
