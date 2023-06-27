// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type NetTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*NetTestRunner)(nil)

func (m *NetTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateNetMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *NetTestRunner) SetupBeforeAgentRun() string {
	err := common.RunCommands([]string{"sudo systemctl restart docker",})
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

func getCommands(roleArn string) []string {
	return []string{
		"mkdir -p " + credsDir,
		"printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\naws_session_token=%s' $(aws sts assume-role --role-arn " + roleArn + " --role-session-name test --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text) | tee " + credsDir + "/credentials>/dev/null",
		"printf '[default]\nregion = us-west-2' > " + credsDir + "/config",
		"printf '[credentials]\n  shared_credential_profile = \"default\"\n  shared_credential_file = \"" + credsDir + "/credentials\"' | sudo tee /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml>/dev/null",
	}
}

func (m *NetTestRunner) GetTestName() string {
	return "Net"
}

func (m *NetTestRunner) GetAgentConfigFileName() string {
	return "net_config.json"
}

func (m *NetTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"net_bytes_sent", "net_bytes_recv", "net_drop_in", "net_drop_out", "net_err_in",
		"net_err_out", "net_packets_sent", "net_packets_recv"}
}

func (m *NetTestRunner) validateNetMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := m.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "interface",
			Value: dimension.ExpectedDimensionValue{aws.String("docker0")},
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
