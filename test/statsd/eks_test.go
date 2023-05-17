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

type EKSRunner struct {
	test_runner.BaseTestRunner
}

func (t *EKSRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))

	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		// there seems to be some delay before the runner is able to fetch metrics from CW
		for j := 0; j < testRetryCount; j++ {
			time.Sleep(15 * time.Second)
			testResult = common.ValidateStatsdMetric(t.DimensionFactory, namespace, "ClusterName", metricName, t.GetAgentRunDuration())
			if testResult.Status == status.SUCCESSFUL {
				break
			}
		}
		testResults[i] = testResult
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EKSRunner) GetTestName() string {
	return "EKSStatsD"
}

func (t *EKSRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *EKSRunner) GetMeasuredMetrics() []string {
	return []string{"statsd_counter_1", "statsd_gauge_1"}
}

func (e *EKSRunner) GetAgentConfigFileName() string {
	return ""
}

var _ test_runner.ITestRunner = (*EKSRunner)(nil)
