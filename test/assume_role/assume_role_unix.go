// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	namespace            = "AssumeRoleTest" // should match whats in agent config file
	agentConfigDirectory = "agent_configs"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
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

	dims := getDimensions(environment.GetEnvironmentMetaData().InstanceId)
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

	err := setupEnvironmentVariables()
	if err != nil {
		return fmt.Errorf("failed to setup environment variables: %w", err)
	}
	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*RoleTestRunner)(nil)

func getDimensions(_ string) []types.Dimension {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}

func Validate(assumeRoleArn string) error {
	return nil
}

func setupEnvironmentVariables() error {

	common.CopyFile("amazon-cloudwatch-agent.service", "/etc/systemd/system/amazon-cloudwatch-agent.service")

	// Test runner does not have sudo permissions, but it can execute sudo commands
	// Use sed to update the PLACEHOLDER value instead of using built-ins
	sedCmd := fmt.Sprintf("sudo sed -i 's/PLACEHOLDER/%s/g' %s", environment.GetEnvironmentMetaData().InstanceArn, "/etc/systemd/system/amazon-cloudwatch-agent.service")
	cmd := exec.Command("bash", "-c", sedCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update amazon-cloudwatch-agent.service file: %s", err)
	}

	return nil
}
