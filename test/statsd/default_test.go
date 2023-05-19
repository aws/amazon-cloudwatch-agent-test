// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"strings"
	"time"
)

const testRetryCount = 3

type StatsDRunner struct {
	test_runner.BaseTestRunner
	testName     string
	dimensionKey string
}

func (t *StatsDRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))

	// ECS taskdef with portMappings has some delays before getting metrics from statsd container
	if strings.Contains(t.testName, "ECS") {
		time.Sleep(60 * time.Second)
	}

	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		for j := 0; j < testRetryCount; j++ {
			testResult = metric.ValidateStatsdMetric(t.DimensionFactory, t.GetTestName(), t.dimensionKey, metricName, metric.StatsdMetricValues[i], t.GetAgentRunDuration(), 1*time.Second)
			if testResult.Status == status.SUCCESSFUL {
				break
			}
			time.Sleep(15 * time.Second)
		}
		testResults[i] = testResult
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *StatsDRunner) GetTestName() string {
	return t.testName
}

func (t *StatsDRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *StatsDRunner) GetMeasuredMetrics() []string {
	return metric.StatsdMetricNames[:2]
}

func (e *StatsDRunner) GetAgentConfigFileName() string {
	return ""
}

var _ test_runner.ITestRunner = (*StatsDRunner)(nil)
