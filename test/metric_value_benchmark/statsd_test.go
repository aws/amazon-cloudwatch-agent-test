// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
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
		testResults[i] = common.ValidateStatsdMetric(t.DimensionFactory, namespace, metricName, t.GetAgentRunDuration())
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
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
	return common.SendStatsdMetrics(2, []string{"key:value"}, time.Second, t.GetAgentRunDuration())
}

func (t *StatsdTestRunner) GetMeasuredMetrics() []string {
	return []string{"statsd_counter_1", "statsd_gauge_1"}
}
