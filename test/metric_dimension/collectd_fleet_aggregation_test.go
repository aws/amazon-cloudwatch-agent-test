// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
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
	metricType := metric.GetCollectDMetricType(metricName)
	ns := "CollectdFleetAggregationTest"

	// Test 1: Fleet-aggregated dimensions (Component + type from aggregation_dimensions)
	fleetSpecs := []DimensionSpec{
		ExactDim("Component", "WebServer"),
		ExactDim("type", metricType),
	}
	result := ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, fleetSpecs)
	if result.Status != status.SUCCESSFUL {
		return result
	}

	// Test 2: Per-instance aggregated dimensions (Component + type + InstanceId)
	instanceSpecs := []DimensionSpec{
		ExactDim("Component", "WebServer"),
		ExactDim("type", metricType),
		{Key: "InstanceId"},
	}
	result = ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, instanceSpecs)
	if result.Status != status.SUCCESSFUL {
		return result
	}

	// Test 3: Host dimension is KEPT (plugin-level append_dimensions does NOT drop host)
	hostSpecs := []DimensionSpec{
		ExactDim("Component", "WebServer"),
		ExactDim("type", metricType),
		HostDim(),
	}
	result = ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, hostSpecs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: plugin-level append_dimensions keeps host dimension and aggregation_dimensions controls published combinations")
	}
	return result
}
