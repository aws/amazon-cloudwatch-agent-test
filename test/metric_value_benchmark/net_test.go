// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
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

func (m *NetTestRunner) GetTestName() string {
	return "Net"
}

func (m *NetTestRunner) GetAgentConfigFileName() string {
	return "net_config.json"
}

func (m *NetTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
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

	fetcher := metric.NewMetricValueFetcher{Env: &environment.MetaData{}, ExpectedDimensionNames: []string{"InstanceId"}}

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
