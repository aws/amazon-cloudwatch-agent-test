// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)

type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	// Stop sender.
	close(metric.Done)
	metricsToFetch := t.GetMeasuredMetrics()
	results := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		results[i] = metric.ValidateStatsdMetric(t.DimensionFactory, namespace, "InstanceId", metricName, metric.StatsdMetricValues[i], t.GetAgentRunDuration(), send_interval)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *StatsdTestRunner) GetTestName() string {
	return "EC2StatsD"
}

func (t *StatsdTestRunner) GetAgentConfigFileName() string {
	return "statsd_config.json"
}

func (t *StatsdTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *StatsdTestRunner) SetupAfterAgentRun() error {
	// Send each metric once a second.
	go metric.SendStatsdMetricsWithEntity()
	return nil
}

func (t *StatsdTestRunner) GetMeasuredMetrics() []string {
	return metric.StatsdMetricNames
}
