// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

// CpuGlobalAppendDimensionsTestRunner tests global append_dimensions for CPU metrics.
//
// When append_dimensions is configured at the GLOBAL level (metrics.append_dimensions),
// the ec2tagger processor is triggered which:
// 1. Adds EC2 metadata dimensions (InstanceId, InstanceType, etc.)
// 2. DROPS the 'host' dimension
//
// This test validates that with global append_dimensions:
// 1. The configured dimensions (InstanceId, InstanceType) ARE added
// 2. The 'host' dimension is DROPPED
type CpuGlobalAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CpuGlobalAppendDimensionsTestRunner)(nil)

func (t *CpuGlobalAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCpuGlobalAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CpuGlobalAppendDimensionsTestRunner) GetTestName() string {
	return "CpuGlobalAppendDimensions"
}

func (t *CpuGlobalAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "cpu_global_append_dimensions.json"
}

func (t *CpuGlobalAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu_time_active"}
}

func (t *CpuGlobalAppendDimensionsTestRunner) validateCpuGlobalAppendDimensionMetric(metricName string) status.TestResult {
	ns := "CpuGlobalAppendDimensionsTest"

	presentSpecs := append(EC2Dims(), ExactDim("cpu", "cpu-total"))
	droppedSpecs := []DimensionSpec{HostDim(), ExactDim("cpu", "cpu-total")}

	result := ValidateGlobalAppendDimensions(&t.BaseTestRunner, ns, metricName, presentSpecs, droppedSpecs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: global append_dimensions adds InstanceId/InstanceType and DROPS host dimension")
	}
	return result
}
