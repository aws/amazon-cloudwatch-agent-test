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

// CollectdNoAppendDimensionsTestRunner tests the baseline behavior of collectd metrics
// WITHOUT any append_dimensions configured. This serves as a negative test case to
// contrast with CollectdAppendDimensionsTestRunner.
//
// Expected behavior without append_dimensions:
// - The 'host' dimension from collectd protocol IS present
// - Custom dimensions like 'InstanceId' are NOT present (not configured)
//
// This test validates that:
// 1. Collectd metrics work without append_dimensions
// 2. The default 'host' dimension from collectd protocol is preserved
// 3. No EC2 metadata dimensions are added unless explicitly configured
type CollectdNoAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectdNoAppendDimensionsTestRunner)(nil)

func (t *CollectdNoAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectdNoAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectdNoAppendDimensionsTestRunner) GetTestName() string {
	return "CollectdNoAppendDimensions"
}

func (t *CollectdNoAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "collectd_no_append_dimensions.json"
}

func (t *CollectdNoAppendDimensionsTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectdNoAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value"}
}

func (t *CollectdNoAppendDimensionsTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *CollectdNoAppendDimensionsTestRunner) validateCollectdNoAppendDimensionMetric(metricName string) status.TestResult {
	metricType := metric.GetCollectDMetricType(metricName)
	ns := "CollectdNoAppendDimensionsTest"

	// Test 1: Without append_dimensions, the 'host' dimension from collectd protocol SHOULD be present.
	hostSpecs := []DimensionSpec{HostDim(), ExactDim("type", metricType)}
	result := ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, hostSpecs)
	if result.Status != status.SUCCESSFUL {
		log.Printf("Expected host dimension to be present without append_dimensions")
		return result
	}

	// Test 2: Verify InstanceId is NOT present (negative test)
	// Without append_dimensions configured, EC2 metadata dimensions should NOT be added
	instanceIdSpecs := []DimensionSpec{{Key: "InstanceId"}, ExactDim("type", metricType)}
	if !ValidateDimensionsAbsent(&t.BaseTestRunner, ns, metricName, instanceIdSpecs) {
		log.Printf("Expected InstanceId dimension to be absent without append_dimensions")
		result.Status = status.FAILED
		return result
	}

	log.Printf("Verified: without append_dimensions, host dimension is present and InstanceId is absent")
	return result
}
