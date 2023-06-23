// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"log"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type NetTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
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

	var (
		dims   []types.Dimension
		failed []dimension.Instruction
	)
	log.Println(fmt.Sprintf("OS VERSION IS %s", m.env.OS))
	if strings.Contains(m.env.OS, "sles") {
		dims, failed = m.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "interface",
				Value: dimension.ExpectedDimensionValue{aws.String("eth0")},
			},
			{
				Key:   "InstanceId",
				Value: dimension.UnknownDimensionValue(),
			},
		})
	} else {
		dims, failed = m.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "interface",
				Value: dimension.ExpectedDimensionValue{aws.String("docker0")},
			},
			{
				Key:   "InstanceId",
				Value: dimension.UnknownDimensionValue(),
			},
		})
	}

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
