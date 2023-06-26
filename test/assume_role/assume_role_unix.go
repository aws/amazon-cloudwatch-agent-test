// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	namespace = "AssumeRoleTest"
	credsDir  = "/tmp/.aws"
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
	err := common.RunCommands(getCommands(environment.GetEnvironmentMetaData().AssumeRoleArn))
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*RoleTestRunner)(nil)

func getCommands(roleArn string) []string {
	return []string{
		"mkdir -p " + credsDir,
		"printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\naws_session_token=%s' $(aws sts assume-role --role-arn " + roleArn + " --role-session-name test --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text) | tee " + credsDir + "/credentials>/dev/null",
		"printf '[default]\nregion = us-west-2' > " + credsDir + "/config",
		"printf '[credentials]\n  shared_credential_profile = \"default\"\n  shared_credential_file = \"" + credsDir + "/credentials\"' | sudo tee /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml>/dev/null",
	}
}

func getDimensions(instanceId string) []types.Dimension {
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
