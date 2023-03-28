// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type EthtoolTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*EthtoolTestRunner)(nil)

func (m *EthtoolTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateEthtoolMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *EthtoolTestRunner) GetTestName() string {
	return "Ethtool"
}

func (m *EthtoolTestRunner) GetAgentConfigFileName() string {
	return "ethtool_config.json"
}

func (m *EthtoolTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"ethtool_queue_0_tx_cnt", "ethtool_queue_0_rx_cnt",
	}
}

func (m *EthtoolTestRunner) validateEthtoolMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return testResult
	}

	var (
		dims   []types.Dimension
		failed []dimension.Instruction
	)

	for _, iface := range ifaces {
		if iface.Name == "eth0" || iface.Name == "ens5" {
			dims, failed = m.DimensionFactory.GetDimensions([]dimension.Instruction{
				{
					Key:   "InstanceId",
					Value: dimension.UnknownDimensionValue(),
				},
				{
					Key:   "driver",
					Value: dimension.ExpectedDimensionValue{aws.String("ena")},
				},
				{
					Key:   "interface",
					Value: dimension.ExpectedDimensionValue{aws.String(iface.Name)},
				},
			})

		}
	}

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)

	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
