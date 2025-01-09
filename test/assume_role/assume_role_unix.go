// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
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
		return err
	}

	agentConfigPath := filepath.Join(agentConfigDirectory, t.AgentConfig.ConfigFileName)
	err = replacePlaceholder(agentConfigPath, environment.GetEnvironmentMetaData().AssumeRoleArn)
	if err != nil {
		return err
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

	f, err := os.OpenFile("/etc/systemd/system/amazon-cloudwatch-agent.service", os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf(`
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# Location: /etc/systemd/system/amazon-cloudwatch-agent.service
# systemctl enable amazon-cloudwatch-agent
# systemctl start amazon-cloudwatch-agent
# systemctl | grep amazon-cloudwatch-agent
# https://www.freedesktop.org/software/systemd/man/systemd.unit.html

[Unit]
Description=Amazon CloudWatch Agent
After=network.target

[Service]
Type=simple
ExecStart=/opt/aws/amazon-cloudwatch-agent/bin/start-amazon-cloudwatch-agent
KillMode=process
Restart=on-failure
RestartSec=60s
Environment="AMZ_SOURCE_ACCOUNT=506463145083"
Environment="AMZ_SOURCE_ARN=%s"

[Install]
WantedBy=multi-user.target
	`, environment.GetEnvironmentMetaData().InstanceArn))
	return err
}

func replacePlaceholder(filename, newValue string) error {
	// Read the entire file
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Replace the placeholder with new value
	newContent := strings.Replace(string(content), "PLACEHOLDER", newValue, -1)

	// Write back to the file
	err = os.WriteFile(filename, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	return nil
}
