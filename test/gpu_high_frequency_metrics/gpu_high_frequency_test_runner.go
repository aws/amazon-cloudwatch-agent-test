// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package gpu_high_frequency_metrics

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/gpu/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type GPUHighFrequencyTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*GPUHighFrequencyTestRunner)(nil)

func (t *GPUHighFrequencyTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult

	// First validate all the metrics that NvidiaTestRunner validates
	// using the common GPU validation logic
	testResults = append(testResults, common.ValidateGPUMetrics(t.env)...)

	// Then validate logs with histogram format (specific to high frequency metrics)
	testResults = append(testResults, common.ValidateHistogramFormat(t.env))

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *GPUHighFrequencyTestRunner) GetTestName() string {
	return t.testName
}

func (t *GPUHighFrequencyTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *GPUHighFrequencyTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *GPUHighFrequencyTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *GPUHighFrequencyTestRunner) SSMParameterName() string {
	return ""
}
