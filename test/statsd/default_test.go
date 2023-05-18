// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
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

	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		// there seems to be some delay before the runner is able to fetch metrics from CW
		for j := 0; j < testRetryCount; j++ {
			testResult = common.ValidateStatsdMetric(t.DimensionFactory, namespace, t.dimensionKey, metricName, common.StatsdMetricValues[i], t.GetAgentRunDuration(), 1*time.Second)
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
	return common.StatsdMetricNames[:2]
}

func (e *StatsDRunner) GetAgentConfigFileName() string {
	return ""
}

var _ test_runner.ITestRunner = (*StatsDRunner)(nil)
