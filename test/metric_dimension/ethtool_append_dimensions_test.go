// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

// EthtoolAppendDimensionsTestRunner tests global append_dimensions for ethtool metrics.
//
// When append_dimensions is configured at the GLOBAL level (metrics.append_dimensions),
// the ec2tagger processor is triggered which:
// 1. Adds EC2 metadata dimensions (InstanceId, InstanceType, etc.)
// 2. DROPS the 'host' dimension
//
// This test validates that with global append_dimensions:
// 1. The configured dimensions (InstanceId, InstanceType) ARE added
// 2. The 'host' dimension is DROPPED
type EthtoolAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*EthtoolAppendDimensionsTestRunner)(nil)

func (t *EthtoolAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEthtoolAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EthtoolAppendDimensionsTestRunner) GetTestName() string {
	return "EthtoolAppendDimensions"
}

func (t *EthtoolAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "ethtool_append_dimensions.json"
}

func (t *EthtoolAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"ethtool_queue_0_tx_cnt"}
}

func (t *EthtoolAppendDimensionsTestRunner) validateEthtoolAppendDimensionMetric(metricName string) status.TestResult {
	testResult := status.TestResult{Name: metricName, Status: status.FAILED}

	ifaceName := getNetworkInterface()
	if ifaceName == "" {
		log.Printf("No suitable network interface found")
		return testResult
	}
	log.Printf("Using network interface: %s", ifaceName)

	ns := "EthtoolAppendDimensionsTest"
	presentSpecs := append(EC2Dims(),
		ExactDim("interface", ifaceName),
		ExactDim("driver", "ena"),
	)
	droppedSpecs := []DimensionSpec{HostDim(), ExactDim("interface", ifaceName)}

	result := ValidateGlobalAppendDimensions(&t.BaseTestRunner, ns, metricName, presentSpecs, droppedSpecs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: global append_dimensions adds InstanceId/InstanceType and DROPS host dimension")
	}
	return result
}
