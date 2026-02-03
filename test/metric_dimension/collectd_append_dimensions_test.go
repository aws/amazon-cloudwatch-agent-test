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

// CollectdAppendDimensionsTestRunner tests plugin-level append_dimensions for collectd.
//
// Per the CloudWatch Agent documentation, plugin-level append_dimensions ADDS dimensions
// to the metrics without dropping existing dimensions like 'host'. This is different from
// global append_dimensions which drops the 'host' dimension.
//
// This test validates that with plugin-level append_dimensions:
// 1. The configured dimensions (InstanceId, InstanceType) ARE added
// 2. The original 'host' dimension from collectd protocol is KEPT (not dropped)
type CollectdAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectdAppendDimensionsTestRunner)(nil)

func (t *CollectdAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectdAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectdAppendDimensionsTestRunner) GetTestName() string {
	return "CollectdAppendDimensions"
}

func (t *CollectdAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "collectd_append_dimensions.json"
}

func (t *CollectdAppendDimensionsTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectdAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value"}
}

func (t *CollectdAppendDimensionsTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *CollectdAppendDimensionsTestRunner) validateCollectdAppendDimensionMetric(metricName string) status.TestResult {
	metricType := metric.GetCollectDMetricType(metricName)
	ns := "CollectdAppendDimensionsTest"

	// With plugin-level append_dimensions, BOTH the configured dimensions AND the original
	// host dimension should be present.
	specs := append(EC2Dims(), HostDim(), ExactDim("type", metricType))

	result := ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, specs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: plugin-level append_dimensions adds InstanceId/InstanceType while keeping host dimension")
	}
	return result
}
