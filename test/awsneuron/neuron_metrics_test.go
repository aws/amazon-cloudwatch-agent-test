// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package awsneuron

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	. "github.com/aws/amazon-cloudwatch-agent-test/test/awsneuron/resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	awsNeuronMetricIndicator = "_neuron"
)

var expectedDimsToMetrics = map[string][]string{
	"ClusterName": {
		NodeNeuronCoreUtil, NodeNeuronCoreMemUsageConstants, NodeNeuronCoreMemUsageModel, NodeNeuronCoreMemUsageScratchpad,
		NodeNeuronCoreMemUsageRuntime, NodeNeuronCoreMemUsageTensors, NodeNeuronCoreMemUsageTotal, NodeNeuronDeviceHwEccEvents,
		NodeExecutionErrorsTotal, NodeNeuronDeviceRuntimeMemoryUsed, NodeNeuronExecutionLatency,
	},
}

type AwsNeuronTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*AwsNeuronTestRunner)(nil)

func (t *AwsNeuronTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, awsNeuronMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	testResults = append(testResults, metric.ValidateLogsFrequency(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AwsNeuronTestRunner) GetTestName() string {
	return t.testName
}

func (t *AwsNeuronTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *AwsNeuronTestRunner) GetAgentRunDuration() time.Duration {
	return 25 * time.Minute
}

func (t *AwsNeuronTestRunner) GetMeasuredMetrics() []string {
	return nil
}
