// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf

import (
	"flag"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	gpuMetricIndicator = "_gpu_"

	containerMemTotal   = "container_gpu_memory_total"
	containerMemUsed    = "container_gpu_memory_used"
	containerPower      = "container_gpu_power_draw"
	containerTemp       = "container_gpu_temperature"
	containerUtil       = "container_gpu_utilization"
	containerMemUtil    = "container_gpu_memory_utilization"
	containerTensorCore = "container_gpu_tensor_core_utilization"
	podMemTotal         = "pod_gpu_memory_total"
	podMemUsed          = "pod_gpu_memory_used"
	podPower            = "pod_gpu_power_draw"
	podTemp             = "pod_gpu_temperature"
	podUtil             = "pod_gpu_utilization"
	podMemUtil          = "pod_gpu_memory_utilization"
	podTensorCore       = "pod_gpu_tensor_core_utilization"
	podLimit            = "pod_gpu_limit"
	podRequest          = "pod_gpu_request"
	podCountTotal       = "pod_gpu_usage_total"
	podReserved         = "pod_gpu_reserved_capacity"
	nodeMemTotal        = "node_gpu_memory_total"
	nodeMemUsed         = "node_gpu_memory_used"
	nodePower           = "node_gpu_power_draw"
	nodeTemp            = "node_gpu_temperature"
	nodeUtil            = "node_gpu_utilization"
	nodeMemUtil         = "node_gpu_memory_utilization"
	nodeTensorCore      = "node_gpu_tensor_core_utilization"
	nodeCountTotal      = "node_gpu_usage_total"
	nodeCountLimit      = "node_gpu_limit"
	nodeReserved        = "node_gpu_reserved_capacity"
)

var useE2EMetrics = flag.Bool("useE2EMetrics", false, "Use E2E metrics mapping which uses latest build CWA")

var expectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		containerMemTotal, containerMemUsed, containerPower, containerTemp, containerUtil, containerMemUtil, containerTensorCore,
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
		nodeMemTotal, nodeMemUsed, nodePower, nodeTemp, nodeUtil, nodeMemUtil, nodeTensorCore,
	},
	"ClusterName-Namespace": {
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
	},
	"ClusterName-Namespace-Service": {
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
	},
	"ClusterName-Namespace-PodName": {
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
	},
	"ClusterName-Namespace-PodName-ContainerName": {
		containerMemTotal, containerMemUsed, containerPower, containerTemp, containerUtil, containerMemUtil, containerTensorCore,
	},
	"ClusterName-Namespace-PodName-FullPodName-ContainerName": {
		containerMemTotal, containerMemUsed, containerPower, containerTemp, containerUtil, containerMemUtil, containerTensorCore,
	},
	"ClusterName-Namespace-PodName-FullPodName-ContainerName-GpuDevice": {
		containerMemTotal, containerMemUsed, containerPower, containerTemp, containerUtil, containerMemUtil, containerTensorCore,
	},
	"ClusterName-Namespace-PodName-FullPodName": {
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
	},
	"ClusterName-Namespace-PodName-FullPodName-GpuDevice": {
		podMemTotal, podMemUsed, podPower, podTemp, podUtil, podMemUtil, podTensorCore,
	},
	"ClusterName-NodeName-InstanceId": {
		nodeMemTotal, nodeMemUsed, nodePower, nodeTemp, nodeUtil, nodeMemUtil, nodeTensorCore,
	},
	"ClusterName-NodeName-InstanceId-InstanceType-GpuDevice": {
		nodeMemTotal, nodeMemUsed, nodePower, nodeTemp, nodeUtil, nodeMemUtil, nodeTensorCore,
	},
}

type NvidiaTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*NvidiaTestRunner)(nil)

func (t *NvidiaTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	expectedDimsToMetrics := expectedDimsToMetricsIntegTest
	if *useE2EMetrics {
		// add GPU count metrics
		expectedDimsToMetricsIntegTest["ClusterName"] = append(expectedDimsToMetricsIntegTest["ClusterName"], podReserved, podRequest, podCountTotal, podLimit, nodeCountTotal, nodeCountLimit, nodeReserved)
		expectedDimsToMetricsIntegTest["ClusterName-Namespace-PodName"] = append(expectedDimsToMetricsIntegTest["ClusterName-Namespace-PodName"], podCountTotal, podRequest, podReserved, podLimit)
		expectedDimsToMetricsIntegTest["ClusterName-Namespace-PodName-FullPodName"] = append(expectedDimsToMetricsIntegTest["ClusterName-Namespace-PodName-FullPodName"], podCountTotal, podRequest, podReserved, podLimit)
		expectedDimsToMetricsIntegTest["ClusterName-NodeName-InstanceId"] = append(expectedDimsToMetricsIntegTest["ClusterName-NodeName-InstanceId"], nodeCountLimit, nodeCountTotal, nodeReserved)
	}
	testResults = append(testResults, metric.ValidateMetrics(t.env, gpuMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *NvidiaTestRunner) GetTestName() string {
	return t.testName
}

func (t *NvidiaTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *NvidiaTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *NvidiaTestRunner) GetMeasuredMetrics() []string {
	return nil
}
