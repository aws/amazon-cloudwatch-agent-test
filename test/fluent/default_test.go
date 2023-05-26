// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const testRetryCount = 3

type FluentRunner struct {
	test_runner.BaseTestRunner
	testName string
}

func (t *FluentRunner) Validate() status.TestGroupResult {
	testResults := make([]status.TestResult, 1)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *FluentRunner) GetTestName() string {
	return t.testName
}

func (t *FluentRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *FluentRunner) GetMeasuredMetrics() []string {
	return []string{}
}

func (e *FluentRunner) GetAgentConfigFileName() string {
	return ""
}

var _ test_runner.ITestRunner = (*FluentRunner)(nil)
