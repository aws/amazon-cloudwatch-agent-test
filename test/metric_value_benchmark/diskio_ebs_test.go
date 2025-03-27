// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type DiskIOEBSTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*DiskIOEBSTestRunner)(nil)

func (m *DiskIOEBSTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateEBSMetric(name)
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *DiskIOEBSTestRunner) GetTestName() string {
	return "DiskIOEBS"
}

func (m *DiskIOEBSTestRunner) GetAgentConfigFileName() string {
	return "diskio_ebs_config.json"
}

func (m *DiskIOEBSTestRunner) SetupBeforeAgentRun() error {
	err := common.RunCommands([]string{"sudo setcap cap_sys_admin+ep /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent"})
	if err != nil {
		return err
	}
	return m.SetUpConfig()
}

func (m *DiskIOEBSTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"diskio_ebs_total_read_ops",
		"diskio_ebs_total_write_ops",
		"diskio_ebs_total_read_bytes",
		"diskio_ebs_total_write_bytes",
		"diskio_ebs_total_read_time",
		"diskio_ebs_total_write_time",
		"diskio_ebs_volume_performance_exceeded_iops",
		"diskio_ebs_volume_performance_exceeded_tp",
		"diskio_ebs_ec2_instance_performance_exceeded_iops",
		"diskio_ebs_ec2_instance_performance_exceeded_tp",
		"diskio_ebs_volume_queue_length",
	}
}

func (m *DiskIOEBSTestRunner) validateEBSMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := m.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "VolumeId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
