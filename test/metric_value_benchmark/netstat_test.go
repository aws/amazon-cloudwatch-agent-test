// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type NetStatTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*NetStatTestRunner)(nil)

func (t *NetStatTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validateNetStatMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *NetStatTestRunner) getTestName() string {
	return "NetStat"
}

func (t *NetStatTestRunner) getAgentConfigFileName() string {
	return "netstat_config.json"
}
func (t *NetStatTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *NetStatTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"netstat_tcp_close":       nil,
		"netstat_tcp_close_wait":  nil,
		"netstat_tcp_closing":     nil,
		"netstat_tcp_established": nil,
		"netstat_tcp_fin_wait1":   nil,
		"netstat_tcp_fin_wait2":   nil,
		"netstat_tcp_last_ack":    nil,
		"netstat_tcp_listen":      nil,
		"netstat_tcp_none":        nil,
		"netstat_tcp_syn_sent":    nil,
		"netstat_tcp_syn_recv":    nil,
		"netstat_tcp_time_wait":   nil,
		"netstat_udp_socket":      nil,
	}
}

func (t *NetStatTestRunner) validateNetStatMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := t.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
