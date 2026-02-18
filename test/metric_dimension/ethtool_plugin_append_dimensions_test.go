// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

// EthtoolPluginAppendDimensionsTestRunner tests plugin-level append_dimensions for ethtool.
//
// Per the CloudWatch Agent documentation, plugin-level append_dimensions ADDS dimensions
// to the metrics without dropping existing dimensions like 'host'. This is different from
// global append_dimensions which drops the 'host' dimension.
//
// This test validates that with plugin-level append_dimensions:
// 1. The configured dimensions (InstanceId, InstanceType) ARE added
// 2. The original 'host' dimension is KEPT (not dropped)
type EthtoolPluginAppendDimensionsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*EthtoolPluginAppendDimensionsTestRunner)(nil)

func (t *EthtoolPluginAppendDimensionsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEthtoolPluginAppendDimensionMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EthtoolPluginAppendDimensionsTestRunner) GetTestName() string {
	return "EthtoolPluginAppendDimensions"
}

func (t *EthtoolPluginAppendDimensionsTestRunner) GetAgentConfigFileName() string {
	return "ethtool_plugin_append_dimensions.json"
}

func (t *EthtoolPluginAppendDimensionsTestRunner) GetMeasuredMetrics() []string {
	return []string{"ethtool_queue_0_tx_cnt"}
}

func (t *EthtoolPluginAppendDimensionsTestRunner) validateEthtoolPluginAppendDimensionMetric(metricName string) status.TestResult {
	testResult := status.TestResult{Name: metricName, Status: status.FAILED}

	ifaceName := getNetworkInterface()
	if ifaceName == "" {
		log.Printf("No suitable network interface found")
		return testResult
	}
	log.Printf("Using network interface: %s", ifaceName)

	ns := "EthtoolPluginAppendDimensionsTest"

	// With plugin-level append_dimensions, BOTH the configured dimensions AND the original
	// host dimension should be present.
	specs := append(EC2Dims(),
		HostDim(),
		ExactDim("interface", ifaceName),
		ExactDim("driver", "ena"),
	)

	result := ValidateDimensionsPresent(&t.BaseTestRunner, ns, metricName, specs)
	if result.Status == status.SUCCESSFUL {
		log.Printf("Verified: plugin-level append_dimensions adds InstanceId/InstanceType while keeping host dimension")
	}
	return result
}
