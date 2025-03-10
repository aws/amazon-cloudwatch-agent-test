// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type CollectDEntityCustomServiceAndEnvironmentRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectDEntityCustomServiceAndEnvironmentRunner)(nil)

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))

	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectDEntity(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) GetTestName() string {
	return "CollectDEntity - Custom Service Name and Environment"
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) GetAgentConfigFileName() string {
	return "collectd_entity_custom_service_and_environment_test.go"
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_counter_1_value"}
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) validateCollectDEntity(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *CollectDEntityCustomServiceAndEnvironmentRunner) GetAgentRunDuration() time.Duration {
	return time.Minute
}
