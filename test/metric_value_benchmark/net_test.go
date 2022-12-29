// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

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
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for name := range metricsToFetch {
		testResults = append(testResults, m.validateNetMetric(name))
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

func (m *NetTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"net_bytes_sent":   nil,
		"net_bytes_recv":   nil,
		"net_drop_in":      nil,
		"net_drop_out":     nil,
		"net_err_in":       nil,
		"net_err_out":      nil,
		"net_packets_sent": nil,
		"net_packets_recv": nil,
	}
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
