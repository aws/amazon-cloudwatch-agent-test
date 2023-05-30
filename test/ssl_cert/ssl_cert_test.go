// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package acceptance

import (
	"fmt"
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	namespace     = "SSLCertTest"
	linuxCertPath = "/etc/pki/tls/certs/ca-bundle.crt"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

type SslCertTestRunner struct {
	test_runner.BaseTestRunner
}

func (t SslCertTestRunner) Validate() status.TestGroupResult {
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

func (t *SslCertTestRunner) validateMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
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

func (t SslCertTestRunner) GetTestName() string {
	return namespace
}

func (t SslCertTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t SslCertTestRunner) GetMeasuredMetrics() []string {
	return []string{"disk_free", "disk_used", "disk_total"}
}

func (t *SslCertTestRunner) SetupBeforeAgentRun() error {
	backupCertPath := linuxCertPath + ".bak"
	commands := []string{
		fmt.Sprintf("sudo mv %s, %s", linuxCertPath, backupCertPath),
		"echo [ssl] | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"
		"echo ca_bundle_path = \\\"" + backupCertPath+ "\\\" | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"
	}

	return common.RunCommands(commands)
}

var _ test_runner.ITestRunner = (*SslCertTestRunner)(nil)

func TestSSLCert(t *testing.T) {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	factory := dimension.GetDimensionFactory(*env)
	runner := test_runner.TestRunner{TestRunner: &SslCertTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("SSL Cert test failed")
		result.Print()
	}
}
