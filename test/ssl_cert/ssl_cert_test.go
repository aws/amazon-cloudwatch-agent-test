// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package ssl

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	namespace = "SSLCertTest"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type SslCertTestRunner struct {
	test_runner.BaseTestRunner
	caCertPath string
}

func (t SslCertTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	time.Sleep(60 * time.Second)
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
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
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
	return metric.CpuMetrics
}

func (t *SslCertTestRunner) SetupBeforeAgentRun() error {
	backupCertPath := t.caCertPath + ".bak"
	commands := []string{
		fmt.Sprintf("sudo mv %s %s", t.caCertPath, backupCertPath),
		"echo [ssl] | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
		"echo ca_bundle_path = \\\"" + backupCertPath + "\\\" | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
	}
	err := common.RunCommands(commands)
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*SslCertTestRunner)(nil)

func TestSSLCert(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	runner := test_runner.TestRunner{TestRunner: &SslCertTestRunner{
		test_runner.BaseTestRunner{DimensionFactory: factory},
		env.CaCertPath,
	}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("SSL Cert test failed")
		result.Print()
	}
}
