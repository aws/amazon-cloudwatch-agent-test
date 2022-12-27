// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type NetStatTestRunner struct {
	test_runner.BaseTestRunner
	Base test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*NetStatTestRunner)(nil)

func (t *NetStatTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateNetStatMetric(metricName)
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
func (t *NetStatTestRunner) GetAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (t *NetStatTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"netstat_tcp_close",
		"netstat_tcp_close_wait",
		"netstat_tcp_closing",
		"netstat_tcp_established",
		"netstat_tcp_fin_wait1",
		"netstat_tcp_fin_wait2",
		"netstat_tcp_last_ack",
		"netstat_tcp_listen",
		"netstat_tcp_none",
		"netstat_tcp_syn_sent",
		"netstat_tcp_syn_recv",
		"netstat_tcp_time_wait",
		"netstat_udp_socket",
	}
}

func (t *NetStatTestRunner) validateNetStatMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.Base.DimensionFactory.GetDimensions([]dimension.Instruction{
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
