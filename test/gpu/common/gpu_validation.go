// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package common

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	GPUMetricIndicator = "_gpu_"

	ContainerMemTotal   = "container_gpu_memory_total"
	ContainerMemUsed    = "container_gpu_memory_used"
	ContainerPower      = "container_gpu_power_draw"
	ContainerTemp       = "container_gpu_temperature"
	ContainerUtil       = "container_gpu_utilization"
	ContainerMemUtil    = "container_gpu_memory_utilization"
	ContainerTensorUtil = "container_gpu_tensor_core_utilization"
	PodMemTotal         = "pod_gpu_memory_total"
	PodMemUsed          = "pod_gpu_memory_used"
	PodPower            = "pod_gpu_power_draw"
	PodTemp             = "pod_gpu_temperature"
	PodUtil             = "pod_gpu_utilization"
	PodMemUtil          = "pod_gpu_memory_utilization"
	PodTensorUtil       = "pod_gpu_tensor_core_utilization"
	PodLimit            = "pod_gpu_limit"
	PodRequest          = "pod_gpu_request"
	PodCountTotal       = "pod_gpu_usage_total"
	PodReserved         = "pod_gpu_reserved_capacity"
	NodeMemTotal        = "node_gpu_memory_total"
	NodeMemUsed         = "node_gpu_memory_used"
	NodePower           = "node_gpu_power_draw"
	NodeTemp            = "node_gpu_temperature"
	NodeUtil            = "node_gpu_utilization"
	NodeMemUtil         = "node_gpu_memory_utilization"
	NodeTensorUtil      = "node_gpu_tensor_core_utilization"
	NodeCountTotal      = "node_gpu_usage_total"
	NodeCountLimit      = "node_gpu_limit"
	NodeReserved        = "node_gpu_reserved_capacity"
	NodeUnreserved      = "node_gpu_unreserved_capacity"
	NodeAvailable       = "node_gpu_available_capacity"
)

var UseE2EMetrics = flag.Bool("useE2EMetrics", false, "Use E2E metrics mapping which uses latest build CWA")

// ExpectedDimsToMetricsIntegTest defines the expected dimensions and metrics for GPU validation
var ExpectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		ContainerMemTotal, ContainerMemUsed, ContainerPower, ContainerTemp, ContainerUtil, ContainerMemUtil, ContainerTensorUtil,
		PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil, PodTensorUtil,
		NodeMemTotal, NodeMemUsed, NodePower, NodeTemp, NodeUtil, NodeMemUtil, NodeTensorUtil,
	},
	"ClusterName-Namespace": {
		PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil, PodTensorUtil,
	},
	//"ClusterName-Namespace-Service": {
	//	PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil,
	//},
	"ClusterName-Namespace-PodName": {
		PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil, PodTensorUtil,
	},
	"ClusterName-ContainerName-Namespace-PodName": {
		ContainerMemTotal, ContainerMemUsed, ContainerPower, ContainerTemp, ContainerUtil, ContainerMemUtil, ContainerTensorUtil,
	},
	"ClusterName-ContainerName-FullPodName-Namespace-PodName": {
		ContainerMemTotal, ContainerMemUsed, ContainerPower, ContainerTemp, ContainerUtil, ContainerMemUtil, ContainerTensorUtil,
	},
	"ClusterName-ContainerName-FullPodName-GpuDevice-Namespace-PodName": {
		ContainerMemTotal, ContainerMemUsed, ContainerPower, ContainerTemp, ContainerUtil, ContainerMemUtil, ContainerTensorUtil,
	},
	"ClusterName-FullPodName-Namespace-PodName": {
		PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil, PodTensorUtil,
	},
	"ClusterName-FullPodName-GpuDevice-Namespace-PodName": {
		PodMemTotal, PodMemUsed, PodPower, PodTemp, PodUtil, PodMemUtil, PodTensorUtil,
	},
	"ClusterName-InstanceId-NodeName": {
		NodeMemTotal, NodeMemUsed, NodePower, NodeTemp, NodeUtil, NodeMemUtil, NodeTensorUtil,
		//NodeCountTotal, NodeCountRequest, NodeCountLimit,
	},
	"ClusterName-GpuDevice-InstanceId-InstanceType-NodeName": {
		NodeMemTotal, NodeMemUsed, NodePower, NodeTemp, NodeUtil, NodeMemUtil, NodeTensorUtil,
	},
}

// ValidateGPUMetrics validates GPU metrics using the common validation logic
func ValidateGPUMetrics(env *environment.MetaData) []status.TestResult {
	var testResults []status.TestResult

	// Create a copy of the expected dimensions to metrics map
	expectedDimsToMetrics := make(map[string][]string)
	for k, v := range ExpectedDimsToMetricsIntegTest {
		expectedDimsToMetrics[k] = append([]string{}, v...)
	}

	// Add GPU count metrics if using E2E metrics
	if *UseE2EMetrics {
		expectedDimsToMetrics["ClusterName"] = append(
			expectedDimsToMetrics["ClusterName"],
			PodReserved, PodRequest, PodCountTotal, PodLimit, NodeCountTotal, NodeCountLimit, NodeReserved, NodeUnreserved, NodeAvailable,
		)
		expectedDimsToMetrics["ClusterName-Namespace-PodName"] = append(
			expectedDimsToMetrics["ClusterName-Namespace-PodName"],
			PodCountTotal, PodRequest, PodReserved, PodLimit,
		)
		expectedDimsToMetrics["ClusterName-FullPodName-Namespace-PodName"] = append(
			expectedDimsToMetrics["ClusterName-FullPodName-Namespace-PodName"],
			PodCountTotal, PodRequest, PodReserved, PodLimit,
		)
		expectedDimsToMetrics["ClusterName-InstanceId-NodeName"] = append(
			expectedDimsToMetrics["ClusterName-InstanceId-NodeName"],
			NodeCountLimit, NodeCountTotal, NodeReserved, NodeUnreserved, NodeAvailable,
		)
	}

	// Validate metrics and logs
	testResults = append(testResults, metric.ValidateMetrics(env, GPUMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(env))

	return testResults
}

// ValidateHistogramFormat validates that the logs contain metrics in histogram format
func ValidateHistogramFormat(env *environment.MetaData) status.TestResult {
	testResult := status.TestResult{
		Name:   "histogram-format",
		Status: status.FAILED,
	}

	end := time.Now()
	start := end.Add(time.Duration(-3) * time.Minute)
	group := fmt.Sprintf("/aws/containerinsights/%s/performance", env.EKSClusterName)

	log.Println("Searching for histogram format in log group:", group)

	// Get the instances used for the EKS cluster
	eKSInstances, err := awsservice.GetEKSInstances(env.EKSClusterName)
	if err != nil {
		log.Println("Failed to get EKS instances:", err)
		return testResult
	}

	histogramFound := false
	logCount := 0
	gpuMetricCount := 0

	for _, instance := range eKSInstances {
		stream := *instance.InstanceName

		err = awsservice.ValidateLogs(
			group,
			stream,
			&start,
			&end,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(
				func(event types.OutputLogEvent) error {
					logCount++
					message := *event.Message

					// Check if the log contains histogram format
					var logData map[string]interface{}
					if err := json.Unmarshal([]byte(message), &logData); err != nil {
						return nil // Skip this log if it's not valid JSON
					}

					// Check for GPU metrics with histogram format
					gpuMetricsInLog := 0
					for key, value := range logData {
						if !strings.Contains(key, "_gpu_") {
							continue
						}

						gpuMetricsInLog++
						gpuMetricCount++

						// Check if the value is a map with histogram fields
						valueMap, ok := value.(map[string]interface{})
						if !ok {
							continue
						}

						// Check for required histogram fields
						_, hasValues := valueMap["Values"]
						_, hasCounts := valueMap["Counts"]
						_, hasMax := valueMap["Max"]
						_, hasMin := valueMap["Min"]
						_, hasCount := valueMap["Count"]
						_, hasSum := valueMap["Sum"]

						if hasValues && hasCounts && hasMax && hasMin && hasCount && hasSum {
							histogramFound = true
							log.Println("Found GPU metric in histogram format:", key)
							return nil
						}
					}

					return nil // Continue checking other logs
				},
			),
		)

		if err != nil {
			log.Println("Error validating logs:", err)
		}

		if histogramFound {
			log.Println("Successfully found GPU metric in histogram format")
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
	}

	log.Printf("Processed %d logs, found %d GPU metrics, but none in histogram format", logCount, gpuMetricCount)
	return testResult
}
