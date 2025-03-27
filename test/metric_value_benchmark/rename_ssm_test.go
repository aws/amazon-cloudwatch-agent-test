// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type RenameSSMTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*RenameSSMTestRunner)(nil)

func (m *RenameSSMTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateMemMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *RenameSSMTestRunner) GetTestName() string {
	return "RenameSSM"
}

func (m *RenameSSMTestRunner) GetAgentConfigFileName() string {
	return "metric_rename_ssm.json"
}

func (m *RenameSSMTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"ssm_cpu_utilization",
	}
}

func (m *RenameSSMTestRunner) validateMemMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := m.DimensionFactory.GetDimensions([]dimension.Instruction{
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

func (m *RenameSSMTestRunner) UseSSM() bool {
	return true
}

func (m *RenameSSMTestRunner) SSMParameterName() string {
	return "MetricRenameSSM"
}
