// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package assume_role

import (
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "AssumeRoleTest"

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

type RoleTestRunner struct {
	test_runner.BaseTestRunner
}

func (t RoleTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *RoleTestRunner) validateMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims := getDimensions(t, envMetaDataStrings.InstanceId)
	if len(dims) == 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t RoleTestRunner) GetTestName() string {
	return namespace
}

func (t RoleTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t RoleTestRunner) GetMeasuredMetrics() []string {
	return metric.CpuMetrics
}

func (t *RoleTestRunner) SetupBeforeAgentRun() error {
	err := common.RunCommands(getCommands(envMetaDataStrings.AssumeRoleArn))
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*RoleTestRunner)(nil)

func TestAssumeRole(t *testing.T) {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	runner := test_runner.TestRunner{TestRunner: &RoleTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("Assume Role Test failed")
		result.Print()
	}
}
