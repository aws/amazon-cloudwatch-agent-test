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
	f := metric.MetricValueFetcher{}
	results := []status.TestResult{}

	for _, testCase := range testCases {
		r := status.TestResult{Name: testCase.metricName, Status: status.SUCCESSFUL}
		instructions := []dimension.Instruction{}
		for _, d := range testCase.dimensions {
			// If dimension value is not explicit, then rely on a provider.
			v := dimension.UnknownDimensionValue()
			if d[1] != "" {
				v = dimension.ExpectedDimensionValue{Value: aws.String(d[1])}
			}
			i := dimension.Instruction{Key: d[0], Value: v}
			instructions = append(instructions, i)
		}
		dd, _ := t.DimensionFactory.GetDimensions(instructions)
		values, err := f.Fetch("TestAggregationDimensions",
			testCase.metricName, dd, metric.AVERAGE,
			metric.HighResolutionStatPeriod)
		if err != nil {
			r.Status = status.FAILED
		} else if testCase.shouldExist && len(values) == 0 {
			log.Printf("error: did not find values for metric %v with dimensions %v",
				testCase.metricName, testCase.dimensions)
			r.Status = status.FAILED
		} else if !testCase.shouldExist && len(values) > 0 {
			log.Printf("error: found values for metric %v with dimensions %v",
				testCase.metricName, testCase.dimensions)
			r.Status = status.FAILED
		}
		results = append(results, r)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *AggregationDimensionsTestRunner) GetTestName() string {
	return "AggregationDimensionsTestRunner"
}

func (t *AggregationDimensionsTestRunner) GetAgentConfigFileName() string {
	return "aggregation_dimensions.json"
}

// GetMeasuredMetrics is not used in this test.
func (t *AggregationDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"mem_used_percent", "cpu_usage_user"}
}

type pair [2]string

type testCase struct {
	shouldExist bool
	metricName  string
	// dimensions is a list of pairs.
	// If the 2nd item in the pair is nil, then rely on a provider get dim val.
	dimensions []pair
}

var testCases = []testCase{
	// Appended dimensions, no aggregation.
	{
		shouldExist: true,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"foo", "fooval"},
			{"bar", "barval"},
			{"baz", "bazval"},
			{"InstanceId", ""},
			{"InstanceType", ""},
		},
	},
	// Aggregate to 0 dimensions.
	{
		shouldExist: true,
		metricName:  "mem_used_percent",
		dimensions:  []pair{},
	},
	// More aggregations...
	{
		shouldExist: true,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"InstanceId", ""},
			{"InstanceType", ""},
		},
	},
	{
		shouldExist: true,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"foo", "fooval"},
			{"bar", "barval"},
			{"InstanceType", ""},
		},
	},
	// Unexpected dimensions.
	{
		shouldExist: false,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"foo", "fooval"},
		},
	},
	{
		shouldExist: false,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"foo", "fooval"},
			{"bar", "barval"},
		},
	},
	{
		shouldExist: false,
		metricName:  "mem_used_percent",
		dimensions: []pair{
			{"InstanceType", ""},
		},
	},
	// Appended dimensions, no aggregation.
	{
		shouldExist: true,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"InstanceId", ""},
			{"InstanceType", ""},
		},
	},
	{
		shouldExist: true,
		metricName:  "cpu_usage_user",
		dimensions:  []pair{},
	},
	{
		shouldExist: true,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"InstanceId", ""},
		},
	},
	// Unexpected dimensions.
	{
		shouldExist: false,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"foo", "fooval"},
		},
	},
	{
		shouldExist: false,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"foo", "fooval"},
			{"bar", "barval"},
		},
	},
	{
		shouldExist: false,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"InstanceType", ""},
		},
	},
	{
		shouldExist: false,
		metricName:  "cpu_usage_user",
		dimensions: []pair{
			{"foo", "fooval"},
			{"bar", "barval"},
			{"baz", "bazval"},
			{"InstanceId", ""},
			{"InstanceType", ""},
		},
	},
}
