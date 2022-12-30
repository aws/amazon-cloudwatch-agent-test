// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validateStatsdMetric(metricName))
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
	return time.Minute
}

func (t *StatsdTestRunner) SetupAfterAgentRun() error {
	// EC2 Image Builder creates a bash script that sends statsd format to cwagent at port 8125
	// The bash script is at /etc/statsd.sh
	//    for times in  {1..3}
	//    do
	//      echo "statsd.counter:1|c" | nc -w 1 -u 127.0.0.1 8125
	//      sleep 60
	//    done
	startStatsdCommand := []string{
		"sudo bash /etc/statsd.sh",
	}

	return common.RunCommands(startStatsdCommand)
}

func (t *StatsdTestRunner) GetMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{"statsd_counter": nil}
}

func (t *StatsdTestRunner) validateStatsdMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{aws.String("counter")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE)
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
