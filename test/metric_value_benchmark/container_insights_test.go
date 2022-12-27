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
	"time"
)

type ContainerInsightsTestRunner struct {
	Base test_runner.BaseTestRunner
}

var _ IECSTestRunner = (*ContainerInsightsTestRunner)(nil)

func (t *ContainerInsightsTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateContainerInsightsMetrics(metricName)
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *ContainerInsightsTestRunner) getTestName() string {
	return "ContainerInstance"
}

func (t *ContainerInsightsTestRunner) getAgentConfigFileName() string {
	return "./agent_configs/container_insights.json"
}

func (t *ContainerInsightsTestRunner) getAgentRunDuration() time.Duration {
	return test_runner.MinimumAgentRuntime
}

func (t *ContainerInsightsTestRunner) getMeasuredMetrics() []string {
	return []string{
		"instance_memory_utilization", "instance_number_of_running_tasks", "instance_memory_reserved_capacity",
		"instance_filesystem_utilization", "instance_network_total_bytes", "instance_cpu_utilization",
		"instance_cpu_reserved_capacity"}
}

func (t *ContainerInsightsTestRunner) validateContainerInsightsMetrics(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.Base.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "ClusterName",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "ContainerInstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("ECS/ContainerInsights", metricName, dims, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}
