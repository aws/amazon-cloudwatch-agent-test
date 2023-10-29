// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//const containerInsightsNamespace = "ContainerInsights"

// list of metrics with more dimensions e.g. PodName and Namespace
//var metricsWithMoreDimensions = []string{"pod_number_of_container_restarts"}

type EKSDaemonTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (e *EKSDaemonTestRunner) Validate() status.TestGroupResult {
	//metrics := e.GetMeasuredMetrics()
	testResults := make([]status.TestResult, 0)
	//for _, name := range metrics {
	//	testResults = append(testResults, e.validateInstanceMetrics(name))
	//}

	testResults = append(testResults, e.validateLogs(e.env))
	return status.TestGroupResult{
		Name:        e.GetTestName(),
		TestResults: testResults,
	}
}

//func (e *EKSDaemonTestRunner) validateInstanceMetrics(name string) status.TestResult {
//	testResult := status.TestResult{
//		Name:   name,
//		Status: status.FAILED,
//	}
//
//	dims, failed := e.DimensionFactory.GetDimensions([]dimension.Instruction{
//		{
//			Key:   "ClusterName",
//			Value: dimension.UnknownDimensionValue(),
//		},
//	})
//	if len(failed) > 0 {
//		log.Println("failed to get dimensions")
//		return testResult
//	}
//
//	// get list of metrics that has more dimensions for container insights
//	// this is to avoid adding more dimension provider for non-trivial dimensions e.g. PodName
//	listFetcher := metric.MetricListFetcher{}
//	if slices.Contains(metricsWithMoreDimensions, name) {
//		metrics, err := listFetcher.Fetch(containerInsightsNamespace, name, dims)
//		if err != nil {
//			log.Println("failed to fetch metric list", err)
//			return testResult
//		}
//
//		if len(metrics) < 1 {
//			log.Println("metric list is empty")
//			return testResult
//		}
//
//		// just verify 1 of returned metrics for values
//		for _, dim := range metrics[0].Dimensions {
//			// skip since it's provided by dimension provider
//			if *dim.Name == "ClusterName" {
//				continue
//			}
//
//			dims = append(dims, types.Dimension{
//				Name:  dim.Name,
//				Value: dim.Value,
//			})
//		}
//	}
//
//	valueFetcher := metric.MetricValueFetcher{}
//	values, err := valueFetcher.Fetch(containerInsightsNamespace, name, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
//	if err != nil {
//		log.Println("failed to fetch metrics", err)
//		return testResult
//	}
//
//	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(name, values, 0) {
//		return testResult
//	}
//
//	testResult.Status = status.SUCCESSFUL
//	return testResult
//}

func (e *EKSDaemonTestRunner) validateLogs(env *environment.MetaData) status.TestResult {
	testResult := status.TestResult{
		Name:   "emf-logs",
		Status: status.FAILED,
	}

	now := time.Now()
	group := fmt.Sprintf("/aws/containerinsights/%s/performance", env.EKSClusterName)

	// need to get the instances used for the EKS cluster
	eKSInstances, err := awsservice.GetEKSInstances(env.EKSClusterName)
	if err != nil {
		log.Println("failed to get EKS instances", err)
		return testResult
	}

	for _, instance := range eKSInstances {
		stream := *instance.InstanceName
		err = awsservice.ValidateLogs(
			group,
			stream,
			nil,
			&now,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(
				awsservice.AssertLogSchema(func(message string) (string, error) {
					var eksClusterType awsservice.EKSClusterType
					innerErr := json.Unmarshal([]byte(message), &eksClusterType)
					if innerErr != nil {
						return "", fmt.Errorf("failed to unmarshal log file: %w", innerErr)
					}

					log.Printf("eksClusterType is: %s", eksClusterType.Type)
					jsonSchema, ok := eks_resources.EksClusterValidationMap[eksClusterType.Type]
					if !ok {
						return "", errors.New("invalid cluster type provided")
					}
					return jsonSchema, nil
				}),
				awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"ClusterName\":\"%s\"", env.EKSClusterName)),
			),
		)

		if err != nil {
			log.Printf("log validation (%s/%s) failed: %v", group, stream, err)
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (e *EKSDaemonTestRunner) GetTestName() string {
	return "EKSContainerInstance"
}

func (e *EKSDaemonTestRunner) GetAgentConfigFileName() string {
	return "" // TODO: maybe not needed?
}

func (e *EKSDaemonTestRunner) GetAgentRunDuration() time.Duration {
	return time.Minute * 3
}

//func (e *EKSDaemonTestRunner) GetMeasuredMetrics() []string {
//	return []string{
//		"cluster_failed_node_count",
//		"cluster_node_count",
//		"namespace_number_of_running_pods",
//		"node_cpu_limit",
//		"node_cpu_reserved_capacity",
//		"node_cpu_usage_total",
//		"node_cpu_utilization",
//		"node_filesystem_utilization",
//		"node_memory_limit",
//		"node_memory_reserved_capacity",
//		"node_memory_utilization",
//		"node_memory_working_set",
//		"node_network_total_bytes",
//		"node_number_of_running_containers",
//		"node_number_of_running_pods",
//		"pod_cpu_reserved_capacity",
//		"pod_cpu_utilization",
//		"pod_cpu_utilization_over_pod_limit",
//		"pod_memory_reserved_capacity",
//		"pod_memory_utilization",
//		"pod_memory_utilization_over_pod_limit",
//		"pod_network_rx_bytes",
//		"pod_network_tx_bytes",
//		"service_number_of_running_pods",
//	}
//}

func (t *EKSDaemonTestRunner) SetAgentConfig(config test_runner.AgentConfig) {}

func (e *EKSDaemonTestRunner) SetupAfterAgentRun() error {
	return nil
}

var _ test_runner.ITestRunner = (*EKSDaemonTestRunner)(nil)
