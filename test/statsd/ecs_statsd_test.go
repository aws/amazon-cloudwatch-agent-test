// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var _ test_runner.IECSTestRunner = (*ECSStatsdTestRunner)(nil)

const testRetryCount = 3

type ECSStatsdTestRunner struct {
	test_runner.ECSBaseTestRunner
}

func (t *ECSStatsdTestRunner) Validate(e *environment.MetaData) status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		result := t.validateStatsdMetric(metricName)

		// there seems to be some delay before the runner is able to fetch metrics from CW
		if result.Status != status.SUCCESSFUL {
			for j := 0; j < testRetryCount; j++ {
				time.Sleep(15 * time.Second)
				result = t.validateStatsdMetric(metricName)
				if result.Status == status.SUCCESSFUL {
					break
				}
			}
		}
		testResults[i] = result
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *ECSStatsdTestRunner) GetTestName() string {
	return "ECSStatsD"
}

func (t *ECSStatsdTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *ECSStatsdTestRunner) GetMeasuredMetrics() []string {
	return []string{"statsd_counter_1"}
}

func (t *ECSStatsdTestRunner) validateStatsdMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	instructions := []dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "key",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("value")},
		},
	}
	switch metricName {
	case "statsd_counter_1":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("counter")},
		})
	case "statsd_gauge_1":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("gauge")},
		})
	}

	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	runDuration := t.GetAgentRunDuration()
	aggregationInterval := 30 * time.Second
	// If aggregation is not happening there could be a data point every 5 seconds.
	// So validate the upper bound.
	upperBound := int(runDuration/aggregationInterval) + 2
	// Allow 2 missing data points in case CW-Metrics-Web-Service has a 1 minute
	// delay to store.
	lowerBound := int(runDuration/aggregationInterval) - 4

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}

	switch metricName {
	case "statsd_counter_1":
		if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 4) {
			return testResult
		}
	case "statsd_gauge_1":
		if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 1) {
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
