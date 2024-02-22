// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"

	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"github.com/qri-io/jsonschema"
)

type EKSDaemonTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (e *EKSDaemonTestRunner) Validate() status.TestGroupResult {
	testResults := make([]status.TestResult, 0)

	// Fetching metrics by cluster dimension
	testMap := e.getMetricsInClusterDimension()

	// Validate metrics availability and data
	for dim, metrics := range eks_resources.DimensionStringToMetricsMap {
		testResults = append(testResults, e.validateMetricsAvailability(dim, metrics, testMap))
		for _, m := range metrics {
			testResults = append(testResults, e.validateMetricData(m, e.translateDimensionStringToDimType(dim)))
		}
	}

	// Validate EMF logs
	testResults = append(testResults, e.validateLogs(e.env))

	// Additional validation for sample count of metrics
	testResults = append(testResults, e.validateSampleCountMetrics())

	return status.TestGroupResult{
		Name:        e.GetTestName(),
		TestResults: testResults,
	}
}

// validateSampleCountMetrics checks if the sample count of specific metrics matches the expected values
func (e *EKSDaemonTestRunner) validateSampleCountMetrics() status.TestResult {
	log.Println("Validating sample count of metrics")

	podCount, err := e.getRunningPodsCount()
	if err != nil {
		log.Println("Error getting running pods count:", err)
		return status.TestResult{Name: "SampleCountValidation", Status: status.FAILED}
	}

	metricName := "pod_cpu_utilization"
	dims := e.translateDimensionStringToDimType("ClusterName")

	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(containerInsightsNamespace, metricName, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
	if err != nil {
		log.Println("Error fetching metric data for", metricName, ":", err)
		return status.TestResult{Name: "SampleCountValidation", Status: status.FAILED}
	}

	for _, value := range values {
		if int(value) != podCount {
			log.Printf("Sample count for %s does not match expected value. Expected: %d, Found: %f\n", metricName, podCount, value)
			return status.TestResult{Name: "SampleCountValidation", Status: status.FAILED}
		}
	}

	return status.TestResult{Name: "SampleCountValidation", Status: status.SUCCESSFUL}
}
func (e *EKSDaemonTestRunner) getRunningPodsCount() (int, error) {
	// Create an in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Error creating in-cluster config: %v\n", err)
		return 0, err
	}

	// Create a clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Error creating clientset: %v\n", err)
		return 0, err
	}

	// Get the list of pods
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing pods: %v\n", err)
		return 0, err
	}

	// Count running pods
	runningPodsCount := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			runningPodsCount++
		}
	}

	return runningPodsCount, nil
}

func (e *EKSDaemonTestRunner) validateInstanceMetrics(name string) status.TestResult {
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}

	dims, failed := e.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "ClusterName",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		log.Println("failed to get dimensions")
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("ContainerInsights", name, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		log.Println("failed to fetch metrics", err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(name, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

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
		validateLogContents := func(s string) bool {
			return strings.Contains(s, fmt.Sprintf("\"ClusterName\":\"%s\"", env.EKSClusterName))
		}

		var ok bool
		stream := *instance.InstanceName
		ok, err = awsservice.ValidateLogs(group, stream, nil, &now, func(logs []string) bool {
			if len(logs) < 1 {
				log.Println(fmt.Sprintf("failed to get logs for instance: %s", stream))
				return false
			}

			for _, l := range logs {
				var eksClusterType awsservice.EKSClusterType
				err := json.Unmarshal([]byte(l), &eksClusterType)
				if err != nil {
					log.Println("failed to unmarshal log file")
				}

				log.Println(fmt.Sprintf("eksClusterType is: %s", eksClusterType.Type))
				jsonSchema, ok := eks_resources.EksClusterValidationMap[eksClusterType.Type]
				if !ok {
					log.Println("invalid cluster type provided")
					return false
				}
				rs := jsonschema.Must(jsonSchema)

				if !awsservice.MatchEMFLogWithSchema(l, rs, validateLogContents) {
					log.Println("failed to match log with schema")
					return false
				}
			}
			return true
		})

		if err != nil || !ok {
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

func (e *EKSDaemonTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"node_cpu_reserved_capacity",
		"node_cpu_utilization",
		"node_network_total_bytes",
		"node_filesystem_utilization",
		"node_number_of_running_pods",
		"node_number_of_running_containers",
		"node_memory_utilization",
		"node_memory_reserved_capacity",
	}
}

func (t *EKSDaemonTestRunner) SetAgentConfig(config test_runner.AgentConfig) {
}

func (e *EKSDaemonTestRunner) SetupBeforeAgentRun() error {
	return nil
}

func (e *EKSDaemonTestRunner) SetupAfterAgentRun() error {
	return nil
}

var _ test_runner.ITestRunner = (*EKSDaemonTestRunner)(nil)

