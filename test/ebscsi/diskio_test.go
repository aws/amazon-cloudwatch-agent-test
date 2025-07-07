// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ebscsi

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	nvmeMetricIndicator = "_diskio_ebs_"

	nvmeReadOpsTotal        = "node_diskio_ebs_total_read_ops"
	nvmeWriteOpsTotal       = "node_diskio_ebs_total_write_ops"
	nvmeReadBytesTotal      = "node_diskio_ebs_total_read_bytes"
	nvmeWriteBytesTotal     = "node_diskio_ebs_total_write_bytes"
	nvmeReadTime            = "node_diskio_ebs_total_read_time"
	nvmeWriteTime           = "node_diskio_ebs_total_write_time"
	nvmeExceededIOPSTime    = "node_diskio_ebs_volume_performance_exceeded_iops"
	nvmeExceededTPTime      = "node_diskio_ebs_volume_performance_exceeded_tp"
	nvmeExceededEC2IOPSTime = "node_diskio_ebs_ec2_instance_performance_exceeded_iops"
	nvmeExceededEC2TPTime   = "node_diskio_ebs_ec2_instance_performance_exceeded_tp"
	nvmeVolumeQueueLength   = "node_diskio_ebs_volume_queue_length"
)

var expectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		nvmeReadOpsTotal, nvmeWriteOpsTotal, nvmeReadBytesTotal, nvmeWriteBytesTotal,
		nvmeReadTime, nvmeWriteTime, nvmeExceededIOPSTime, nvmeExceededTPTime,
		nvmeExceededEC2IOPSTime, nvmeExceededEC2TPTime, nvmeVolumeQueueLength,
	},
	"ClusterName-InstanceId-NodeName": {
		nvmeReadOpsTotal, nvmeWriteOpsTotal, nvmeReadBytesTotal, nvmeWriteBytesTotal,
		nvmeReadTime, nvmeWriteTime, nvmeExceededIOPSTime, nvmeExceededTPTime,
		nvmeExceededEC2IOPSTime, nvmeExceededEC2TPTime, nvmeVolumeQueueLength,
	},
	"ClusterName-InstanceId-NodeName-VolumeId": {
		nvmeReadOpsTotal, nvmeWriteOpsTotal, nvmeReadBytesTotal, nvmeWriteBytesTotal,
		nvmeReadTime, nvmeWriteTime, nvmeExceededIOPSTime, nvmeExceededTPTime,
		nvmeExceededEC2IOPSTime, nvmeExceededEC2TPTime, nvmeVolumeQueueLength,
	},
}

type DiskIOTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*DiskIOTestRunner)(nil)

func (t *DiskIOTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, nvmeMetricIndicator, expectedDimsToMetricsIntegTest)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *DiskIOTestRunner) GetTestName() string {
	return t.testName
}

func (t *DiskIOTestRunner) GetAgentConfigFileName() string {
	return "./resources/config.json"
}

func (t *DiskIOTestRunner) GetAgentRunDuration() time.Duration {
	return 5 * time.Minute
}

func (t *DiskIOTestRunner) GetMeasuredMetrics() []string {
	return nil
}
