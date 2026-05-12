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

// CollectdGlobalAppendDimensionsTestRunner tests global append_dimensions for collectd.
//
// When append_dimensions is configured at the GLOBAL level (metrics.append_dimensions),
// the ec2tagger processor is triggered which:
// 1. Adds EC2 metadata dimensions (InstanceId, InstanceType, etc.)
// 2. DROPS the 'host' dimension
//
// This is different from plugin-level append_dimensions which keeps the host dimension.
type CollectdGlobalAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectdGlobalAppendDimensionsTestRunner)(nil)

func (t *CollectdGlobalAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectdGlobalAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectdGlobalAppendDimensionsTestRunner) GetTestName() string {
	return "CollectdGlobalAppendDimensions"
}

func (t *CollectdGlobalAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "collectd_global_append_dimensions.json"
}

func (t *CollectdGlobalAppendDimensionsTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectdGlobalAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value"}
}

func (t *CollectdGlobalAppendDimensionsTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *CollectdGlobalAppendDimensionsTestRunner) validateCollectdGlobalAppendDimensionMetric(metricName string) status.TestResult {
	metricType := metric.GetCollectDMetricType(metricName)
	ns := "CollectdGlobalAppendDimensionsTest"

	presentSpecs := append(EC2Dims(), ExactDim("type", metricType))
	droppedSpecs := []DimensionSpec{HostDim(), ExactDim("type", metricType)}

	result := ValidateGlobalAppendDimensions(&t.BaseTestRunner, ns, metricName, presentSpecs, droppedSpecs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: global append_dimensions adds InstanceId/InstanceType and DROPS host dimension")
	}
	return result
}
