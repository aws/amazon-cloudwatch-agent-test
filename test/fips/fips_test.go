// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package fips

import (
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	namespace            = "FIPSTest"
	agentRuntime         = 3 * time.Minute
	agentConfigLocalPath = "agent_configs/config.json"
	agentConfigPath      = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestFIPS(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)

	common.CopyFile(agentConfigLocalPath, agentConfigPath)
	err := common.StartAgent(agentConfigPath, false, false)
	if err != nil {
		log.Printf("Agent failed to start due to err=%v\n", err)
	}
	time.Sleep(agentRuntime)
	common.StopAgent()

	for _, metric := range metric.CpuMetrics {
		result := validateCpuMetric(metric, factory)
		if result.Status != status.SUCCESSFUL {
			t.Fatalf("FIPS metric validation failed with %s", metric)
		}
	}
}

func validateCpuMetric(metricName string, factory dimension.Factory) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("metric values are %v", values)
	if err != nil {
		log.Printf("err: %v\n", err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
