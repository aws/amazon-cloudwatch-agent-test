// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type StatsdTestRunner struct {
}

var _ ITestRunner = (*StatsdTestRunner)(nil)

func (t *StatsdTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = validateStatsdMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *StatsdTestRunner) getTestName() string {
	return "Statsd"
}

func (t *StatsdTestRunner) getAgentConfigFileName() string {
	return "statsd_config.json"
}

func (t *StatsdTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *StatsdTestRunner) setupBeforeAgentRun() error {
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

func (t *StatsdTestRunner) getMeasuredMetrics() []string {
	return []string{"statsd_counter"}
}

func validateStatsdMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := metric.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
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
