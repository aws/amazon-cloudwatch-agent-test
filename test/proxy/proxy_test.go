// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package proxy

import (
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	namespace = "ProxyTest"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

type ProxyTestRunner struct {
	test_runner.BaseTestRunner
	proxyUrl string
}

func (t ProxyTestRunner) Validate() status.TestGroupResult {
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

func (t *ProxyTestRunner) validateMetric(metricName string) status.TestResult {
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

func (t ProxyTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t ProxyTestRunner) GetTestName() string {
	return namespace
}

func (t ProxyTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t ProxyTestRunner) GetMeasuredMetrics() []string {
	return metric.CpuMetrics
}

func (t *ProxyTestRunner) SetupBeforeAgentRun() error {
	return common.RunCommands(GetCommandToCreateProxyConfig(t.proxyUrl))
}

var _ test_runner.ITestRunner = (*ProxyTestRunner)(nil)

func TestProxy(t *testing.T) {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	factory := dimension.GetDimensionFactory(*env)
	runner := test_runner.TestRunner{TestRunner: &ProxyTestRunner{
		test_runner.BaseTestRunner{DimensionFactory: factory},
		env.ProxyUrl,
	}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("Proxy test failed")
		result.Print()
	}
}
