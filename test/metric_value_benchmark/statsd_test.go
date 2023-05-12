// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)

type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateStatsdMetric(metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *StatsdTestRunner) GetTestName() string {
	return "Statsd"
}

func (t *StatsdTestRunner) GetAgentConfigFileName() string {
	return "statsd_config.json"
}

func (t *StatsdTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *StatsdTestRunner) SetupAfterAgentRun() error {
	return common.SendStatsdMetrics(3, []string{"key:value"}, time.Second, t.GetAgentRunDuration())
}

func (t *StatsdTestRunner) GetMeasuredMetrics() []string {
	return []string{"statsd_counter_1", "statsd_gauge_1"}
}

func (t *StatsdTestRunner) validateStatsdMetric(metricName string) status.TestResult {
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
	lowerBound := int(runDuration/aggregationInterval) - 2

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}

	switch metricName {
	case "statsd_counter_1":
		if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 5) {
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
