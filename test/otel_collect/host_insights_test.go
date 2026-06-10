// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	hostInsightsNamespace = "CWAgent"
	hostInsightsRuntime   = 2 * time.Minute
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type HostInsightsTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*HostInsightsTestRunner)(nil)

func (t *HostInsightsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateHostMetric(metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *HostInsightsTestRunner) validateHostMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.ExpectedDimensionValue{Value: aws.String(t.env.InstanceId)},
		},
	})
	if len(failed) > 0 {
		testResult.Reason = fmt.Errorf("failed to get dimensions for %s", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(hostInsightsNamespace, metricName, dims, metric.AVERAGE, metric.MinuteStatPeriod)
	if err != nil {
		testResult.Reason = err
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		testResult.Reason = fmt.Errorf("metric %s values not valid", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *HostInsightsTestRunner) GetTestName() string             { return "HostInsights" }
func (t *HostInsightsTestRunner) GetAgentRunDuration() time.Duration { return hostInsightsRuntime }
func (t *HostInsightsTestRunner) GetAgentConfigFileName() string  { return "host_insights_config.json" }
func (t *HostInsightsTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"cpu_usage_idle",
		"mem_used_percent",
		"disk_used_percent",
		"net_bytes_sent",
		"diskio_reads",
		"cpu_usage_system",
		"mem_available_percent",
	}
}

func TestHostInsights(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)

	testRunner := &HostInsightsTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{DimensionFactory: factory},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
