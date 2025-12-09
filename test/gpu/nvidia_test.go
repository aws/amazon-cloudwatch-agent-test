// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/gpu/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type NvidiaTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*NvidiaTestRunner)(nil)

func (t *NvidiaTestRunner) Validate() status.TestGroupResult {
	// Use the common GPU validation logic
	testResults := common.ValidateGPUMetrics(t.env)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *NvidiaTestRunner) GetTestName() string {
	return t.testName
}

func (t *NvidiaTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *NvidiaTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *NvidiaTestRunner) GetMeasuredMetrics() []string {
	return nil
}
