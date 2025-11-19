// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package liscsi

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	nvmeInstanceStoreMetricIndicator = "_diskio_instance_store_"

	nvmeInstanceStoreReadOpsTotal        = "node_diskio_instance_store_total_read_ops"
	nvmeInstanceStoreWriteOpsTotal       = "node_diskio_instance_store_total_write_ops"
	nvmeInstanceStoreReadBytesTotal      = "node_diskio_instance_store_total_read_bytes"
	nvmeInstanceStoreWriteBytesTotal     = "node_diskio_instance_store_total_write_bytes"
	nvmeInstanceStoreReadTime            = "node_diskio_instance_store_total_read_time"
	nvmeInstanceStoreWriteTime           = "node_diskio_instance_store_total_write_time"
	nvmeInstanceStoreExceededIOPS        = "node_diskio_instance_store_ec2_instance_performance_exceeded_iops"
	nvmeInstanceStoreExceededTP          = "node_diskio_instance_store_ec2_instance_performance_exceeded_tp"
	nvmeInstanceStoreVolumeQueueLength   = "node_diskio_instance_store_volume_queue_length"
)

var expectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		nvmeInstanceStoreReadOpsTotal, nvmeInstanceStoreWriteOpsTotal, nvmeInstanceStoreReadBytesTotal, nvmeInstanceStoreWriteBytesTotal,
		nvmeInstanceStoreReadTime, nvmeInstanceStoreWriteTime, nvmeInstanceStoreExceededIOPS, nvmeInstanceStoreExceededTP,
		nvmeInstanceStoreVolumeQueueLength,
	},
	"ClusterName-InstanceId-NodeName": {
		nvmeInstanceStoreReadOpsTotal, nvmeInstanceStoreWriteOpsTotal, nvmeInstanceStoreReadBytesTotal, nvmeInstanceStoreWriteBytesTotal,
		nvmeInstanceStoreReadTime, nvmeInstanceStoreWriteTime, nvmeInstanceStoreExceededIOPS, nvmeInstanceStoreExceededTP,
		nvmeInstanceStoreVolumeQueueLength,
	},
	"ClusterName-InstanceId-NodeName-VolumeId": {
		nvmeInstanceStoreReadOpsTotal, nvmeInstanceStoreWriteOpsTotal, nvmeInstanceStoreReadBytesTotal, nvmeInstanceStoreWriteBytesTotal,
		nvmeInstanceStoreReadTime, nvmeInstanceStoreWriteTime, nvmeInstanceStoreExceededIOPS, nvmeInstanceStoreExceededTP,
		nvmeInstanceStoreVolumeQueueLength,
	},
}

type LISCSITestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*LISCSITestRunner)(nil)

func (t *LISCSITestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, nvmeInstanceStoreMetricIndicator, expectedDimsToMetricsIntegTest)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *LISCSITestRunner) GetTestName() string {
	return t.testName
}

func (t *LISCSITestRunner) GetAgentConfigFileName() string {
	return "./resources/config.json"
}

func (t *LISCSITestRunner) GetAgentRunDuration() time.Duration {
	return 5 * time.Minute
}

func (t *LISCSITestRunner) GetMeasuredMetrics() []string {
	return nil
}
