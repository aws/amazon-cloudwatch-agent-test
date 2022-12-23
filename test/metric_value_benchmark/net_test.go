// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows
// +build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type NetTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*NetTestRunner)(nil)

func (m *NetTestRunner) validate() status.TestGroupResult {
	metricsToFetch := m.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateNetMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.getTestName(),
		TestResults: testResults,
	}
}

func (m *NetTestRunner) getTestName() string {
	return "Net"
}

func (m *NetTestRunner) getAgentConfigFileName() string {
	return "net_config.json"
}

func (m *NetTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (m *NetTestRunner) setupAfterAgentRun() error {
	return nil
}

func (m *NetTestRunner) getMeasuredMetrics() []string {
	return []string{
		"net_bytes_sent", "net_bytes_recv", "net_drop_in", "net_drop_out", "net_err_in",
		"net_err_out", "net_packets_sent", "net_packets_recv"}
}

func (m *NetTestRunner) validateNetMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := m.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

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
