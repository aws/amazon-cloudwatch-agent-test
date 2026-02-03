// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

// CollectdFleetAggregationTestRunner tests fleet aggregation with plugin-level append_dimensions.
//
// This test uses plugin-level append_dimensions (under metrics_collected.collectd.append_dimensions)
// which ADDS dimensions without dropping the 'host' dimension. The aggregation_dimensions config
// controls which dimension combinations are published to CloudWatch.
//
// The test validates:
// 1. Aggregated metrics with [Component, type] dimensions exist (fleet-wide aggregation)
// 2. Aggregated metrics with [Component, type, InstanceId] dimensions exist (per-instance)
// 3. The host dimension is KEPT (plugin-level behavior) - verified via aggregation_dimensions
type CollectdFleetAggregationTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectdFleetAggregationTestRunner)(nil)

func (t *CollectdFleetAggregationTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectdFleetAggregationMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectdFleetAggregationTestRunner) GetTestName() string {
	return "CollectdFleetAggregation"
}

func (t *CollectdFleetAggregationTestRunner) GetAgentConfigFileName() string {
	return "collectd_fleet_aggregation.json"
}

func (t *CollectdFleetAggregationTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectdFleetAggregationTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value"}
}

func (t *CollectdFleetAggregationTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *CollectdFleetAggregationTestRunner) validateCollectdFleetAggregationMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	metricType := metric.GetCollectDMetricType(metricName)
	fetcher := metric.MetricValueFetcher{}

	// Test 1: Verify fleet-aggregated dimensions exist (Component + type from aggregation_dimensions)
	// This is the fleet-wide aggregation without per-host breakdown
	fleetDims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "Component",
			Value: dimension.ExpectedDimensionValue{aws.String("WebServer")},
		},
		{
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{aws.String(metricType)},
		},
	})

	if len(failed) > 0 {
		log.Printf("Failed to get fleet dimensions: %v", failed)
		return testResult
	}

	values, err := fetcher.Fetch("CollectdFleetAggregationTest", metricName, fleetDims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("Fleet aggregated metric values (Component + type) are %v", values)
	if err != nil {
		log.Printf("Error fetching fleet metrics: %v", err)
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		log.Printf("Expected fleet aggregated metrics but none found")
		return testResult
	}

	// Test 2: Verify per-instance aggregated dimensions exist (Component + type + InstanceId)
	instanceDims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "Component",
			Value: dimension.ExpectedDimensionValue{aws.String("WebServer")},
		},
		{
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{aws.String(metricType)},
		},
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		log.Printf("Failed to get instance dimensions: %v", failed)
		return testResult
	}

	values, err = fetcher.Fetch("CollectdFleetAggregationTest", metricName, instanceDims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("Per-instance metric values (Component + type + InstanceId) are %v", values)
	if err != nil {
		log.Printf("Error fetching per-instance metrics: %v", err)
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		log.Printf("Expected per-instance aggregated metrics but none found")
		return testResult
	}

	// Test 3: Verify host dimension is KEPT by checking metrics with host in aggregation_dimensions
	// Plugin-level append_dimensions does NOT drop host, so we should find metrics with host
	hostDims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "Component",
			Value: dimension.ExpectedDimensionValue{aws.String("WebServer")},
		},
		{
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{aws.String(metricType)},
		},
		{
			Key:   "host",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		log.Printf("Failed to get host dimensions: %v", failed)
		return testResult
	}

	values, err = fetcher.Fetch("CollectdFleetAggregationTest", metricName, hostDims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("Metrics with host dimension (Component + type + host) are %v", values)
	if err != nil {
		log.Printf("Error fetching metrics with host: %v", err)
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		log.Printf("Expected metrics with host dimension (plugin-level keeps host) but none found")
		return testResult
	}

	log.Printf("Verified: plugin-level append_dimensions keeps host dimension and aggregation_dimensions controls published combinations")
	testResult.Status = status.SUCCESSFUL
	return testResult
}
