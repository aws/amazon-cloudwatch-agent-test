// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type EKSDeploymentTestRunner struct {
	test_runner.BaseTestRunner
}

var _ IEKSTestRunner = (*EKSDeploymentTestRunner)(nil)

func (e *EKSDeploymentTestRunner) validate(*environment.MetaData) status.TestGroupResult {
	metrics := e.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metrics))
	for _, name := range metrics {
		testResults = append(testResults, e.validateInstanceMetrics(name))
	}

	return status.TestGroupResult{
		Name:        e.getTestName(),
		TestResults: testResults,
	}
}

func (e *EKSDeploymentTestRunner) validateInstanceMetrics(name string) status.TestResult {
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}

	dims, failed := e.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "ClusterName",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		log.Println("failed to get dimensions")
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("MetricValueBenchmarkTest", name, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		log.Println("failed to fetch metrics", err)
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToExpectedValue(name, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (e *EKSDeploymentTestRunner) getTestName() string {
	return "EKSPrometheus"
}

func (e *EKSDeploymentTestRunner) getAgentConfigFileName() string {
	return "" // TODO: maybe not needed?
}

func (e *EKSDeploymentTestRunner) getAgentRunDuration() time.Duration {
	return time.Minute * 3
}

func (e *EKSDeploymentTestRunner) getMeasuredMetrics() []string {
	return []string{
		"redis_net_(in|out)put_bytes_total",
		"redis_(expired|evicted)_keys_total",
		"redis_keyspace_(hits|misses)_total",
		"redis_memory_used_bytes",
		"redis_connected_clients",
	}
}
