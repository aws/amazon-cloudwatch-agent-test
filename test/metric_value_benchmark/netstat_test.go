// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
)

type NetStatTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*NetStatTestRunner)(nil)

func (t *NetStatTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validateNetStatMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *NetStatTestRunner) GetTestName() string {
	return "NetStat"
}

func (t *NetStatTestRunner) GetAgentConfigFileName() string {
	return "netstat_config.json"
}

func (t *NetStatTestRunner) GetMeasuredMetrics() map[string]*metric.Bounds {
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
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE)

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
