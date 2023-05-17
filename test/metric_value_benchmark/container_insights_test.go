// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/qri-io/jsonschema"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type ContainerInsightsTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

//go:embed agent_resources/container_insights_node_telemetry.json
var emfContainerInsightsSchema string

var _ test_runner.ITestRunner = (*ContainerInsightsTestRunner)(nil)

func (t *ContainerInsightsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateContainerInsightsMetrics(metricName)
	}

	testResults = append(testResults, validateLogsForContainerInsights(t.env))

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *ContainerInsightsTestRunner) GetTestName() string {
	return "ContainerInstance"
}

func (t *ContainerInsightsTestRunner) GetAgentConfigFileName() string {
	return "./agent_configs/container_insights.json"
}

func (t *ContainerInsightsTestRunner) getAgentRunDuration() time.Duration {
	return time.Minute
}

func (t *ContainerInsightsTestRunner) GetMeasuredMetrics() []string {
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

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
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
	values, err := fetcher.Fetch("ECS/ContainerInsights", metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func validateLogsForContainerInsights(e *environment.MetaData) status.TestResult {
	testResult := status.TestResult{
		Name:   "emf-logs",
		Status: status.FAILED,
	}

	rs := jsonschema.Must(emfContainerInsightsSchema)

	now := time.Now()
	group := fmt.Sprintf("/aws/ecs/containerinsights/%s/performance", e.EcsClusterName)

	// need to derive the container Instance ID first
	containers, err := awsservice.GetContainerInstances(e.EcsClusterArn)
	if err != nil {
		return testResult
	}

	for _, container := range containers {
		validateLogContents := func(s string) bool {
			return strings.Contains(s, fmt.Sprintf("\"ContainerInstanceId\":\"%s\"", container.ContainerInstanceId))
		}

		var ok bool
		stream := fmt.Sprintf("NodeTelemetry-%s", container.ContainerInstanceId)
		ok, err = awsservice.ValidateLogs(group, stream, nil, &now, func(logs []string) bool {
			if len(logs) < 1 {
				return false
			}

			for _, l := range logs {
				if !awsservice.MatchEMFLogWithSchema(l, rs, validateLogContents) {
					return false
				}
			}
			return true
		})

		if err != nil || !ok {
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
