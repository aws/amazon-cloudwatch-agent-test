// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"time"
)

type MemTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*MemTestRunner)(nil)

func (m *MemTestRunner) Validate() status.TestGroupResult {
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

func (m *MemTestRunner) GetTestName() string {
	return "Mem"
}

func (m *MemTestRunner) GetAgentConfigFileName() string {
	return "mem_config.json"
}

func (m *MemTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (m *MemTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"mem_active", "mem_available", "mem_available_percent", "mem_buffered", "mem_cached",
		"mem_free", "mem_inactive", "mem_total", "mem_used", "mem_used_percent"}
}

func (m *MemTestRunner) validateMemMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher := metric.MetricValueFetcher{Env: &environment.MetaData{}, ExpectedDimensionNames: []string{"InstanceId"}}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
