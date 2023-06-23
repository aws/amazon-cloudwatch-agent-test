// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package userdata

import (
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "UserdataTest"

type UserdataTestRunner struct {
	test_runner.BaseTestRunner
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func (t UserdataTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCpuMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *UserdataTestRunner) validateCpuMetric(metricName string) status.TestResult {
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
		log.Printf("err: %v\n", err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t UserdataTestRunner) GetTestName() string {
	return namespace
}

func (t UserdataTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t UserdataTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active_userdata"}
}

func (t UserdataTestRunner) Run() status.TestGroupResult {
	testName := t.GetTestName()
	log.Printf("Running %v", testName)

	log.Printf("Running test for runAgent in userdata mode")
	testGroupResult := status.TestGroupResult{
		Name: t.GetTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.SUCCESSFUL,
			},
		},
	}

	testGroupResult = t.Validate()
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test run failed", testName)
	}

	return testGroupResult
}

func TestUserdata(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	// userdata doesn't use Run() from base_test_runner since agent has already been started with userdata script
	userdataRunner := &UserdataTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}
	result := userdataRunner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("Userdata test failed")
		result.Print()
	}
}

var _ test_runner.ITestRunner = (*UserdataTestRunner)(nil)
