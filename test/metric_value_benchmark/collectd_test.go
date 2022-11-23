// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/agent"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type CollectDTestRunner struct {
}

var _ ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = validateCollectDMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDTestRunner) getTestName() string {
	return "CollectD"
}

func (t *CollectDTestRunner) getAgentConfigFileName() string {
	return "collectd_config.json"
}

func (t *CollectDTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *CollectDTestRunner) setupBeforeAgentRun() error {
	startCollectdCommands := []string{
		"sudo mkdir -p /etc/collectd",
		"sudo collectd -C ./extra_configs/collectd.conf",
	}

	return agent.RunCommands(startCollectdCommands)
}

func (t *CollectDTestRunner) getMeasuredMetrics() []string {
	return []string{"collectd_cpu_value"}
}

func validateCollectDMetric(metricName string) status.TestResult {
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
