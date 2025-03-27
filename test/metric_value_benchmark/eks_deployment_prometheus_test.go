// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type EKSDeploymentTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (e *EKSDeploymentTestRunner) Validate() status.TestGroupResult {
	metrics := e.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metrics))
	for _, name := range metrics {
		testResults = append(testResults, e.validateInstanceMetrics(name))
	}

	return status.TestGroupResult{
		Name:        e.GetTestName(),
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
			Value: dimension.ExpectedDimensionValue{Value: aws.String(e.env.EKSClusterName)},
		},
		{
			Key:   "Namespace",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("redis-test")},
		},
	})

	if len(failed) > 0 {
		log.Println("failed to get dimensions")
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("ContainerInsights/Prometheus", name, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		log.Println("failed to fetch metrics", err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(name, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (e *EKSDeploymentTestRunner) GetTestName() string {
	return "EKSPrometheus"
}

func (e *EKSDeploymentTestRunner) GetAgentConfigFileName() string {
	return "" // TODO: maybe not needed?
}

func (e *EKSDeploymentTestRunner) GetAgentRunDuration() time.Duration {
	return time.Minute * 3
}

func (e *EKSDeploymentTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"redis_net_input_bytes_total",
		"redis_net_output_bytes_total",
		"redis_expired_keys_total",
		"redis_evicted_keys_total",
		"redis_keyspace_hits_total",
		"redis_keyspace_misses_total",
		"redis_memory_used_bytes",
		"redis_connected_clients",
	}
}
