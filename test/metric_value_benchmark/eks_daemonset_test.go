// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const containerInsightsNamespace = "ContainerInsights"

// list of metrics with more dimensions e.g. PodName and Namespace
var metricsWithMoreDimensions = []string{"pod_number_of_container_restarts"}

type EKSDaemonTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

func (e *EKSDaemonTestRunner) Validate() status.TestGroupResult {
	testResults := make([]status.TestResult, 0)
	testMap := e.getMetricsInClusterDimension()
	for dim, metrics := range eks_resources.DimensionStringToMetricsMap {
		testResults = append(testResults, e.validateMetricsAvailability(dim, metrics, testMap))
		for _, m := range metrics {
			testResults = append(testResults, e.validateMetricData(m, e.translateDimensionStringToDimType(dim)))
		}
	}
	testResults = append(testResults, e.validateLogs(e.env))
	return status.TestGroupResult{
		Name:        e.GetTestName(),
		TestResults: testResults,
	}
}

type void struct{}

func (e *EKSDaemonTestRunner) getMetricsInClusterDimension() map[string]map[string]void {
	listFetcher := metric.MetricListFetcher{}
	log.Printf("Fetching by cluster dimension")
	actualMetrics, err := listFetcher.FetchByDimension(containerInsightsNamespace, e.translateDimensionStringToDimType("ClusterName"))
	if err != nil {
		log.Println("failed to fetch metric list", err)
		return nil
	}

	if len(actualMetrics) < 1 {
		log.Println("cloudwatch metric list is empty")
		return nil
	}
	log.Printf("length of metrics %d", len(actualMetrics))
	testMap := make(map[string]map[string]void)
	for _, m := range actualMetrics {
		var s string
		for i, d := range m.Dimensions {
			if i == len(m.Dimensions)-1 {
				s += *d.Name
				break
			}
			s += *d.Name + "-"
		}
		log.Printf("for dimension string %s", s)
		if testMap == nil || testMap[s] == nil {
			mtr := make(map[string]void)
			mtr[*m.MetricName] = void{}
			testMap[s] = mtr
		} else {
			testMap[s][*m.MetricName] = void{}
		}
	}

	return testMap
}

func logMetrics(metrics map[string]void, dim string) {
	log.Printf("\t dimension: %s, Metrics:\n", dim)
	for d, _ := range metrics {
		log.Printf("metric name: %s", d)
	}
}

func (e *EKSDaemonTestRunner) validateMetricsAvailability(dimensionString string, metrics []string, testMap map[string]map[string]void) status.TestResult {
	log.Printf("validateMetricsAvailability for dimension: %v", dimensionString)
	testResult := status.TestResult{
		Name:   dimensionString,
		Status: status.FAILED,
	}
	actualMetrics := testMap[dimensionString]

	//verify the result metrics with expected metrics
	log.Printf("length of actual metrics %d", len(actualMetrics))
	log.Printf("length of expected metrics %d", len(metrics))
	logMetrics(actualMetrics, dimensionString)
	if compareMaptoList(actualMetrics, metrics) {
		testResult.Status = status.SUCCESSFUL
	}
	return testResult
}

func (e *EKSDaemonTestRunner) translateDimensionStringToDimType(dimensionString string) []types.Dimension {
	split := strings.Split(dimensionString, "-")
	var dims []types.Dimension
	for _, str := range split {
		dims = append(dims, types.Dimension{
			Name:  aws.String(str),
			Value: aws.String(e.getDimensionValue(str)),
		})
		log.Printf("dim key %s", str)
		log.Printf("dim value %s", e.getDimensionValue(str))
	}
	log.Printf("dimensions length %d", len(dims))
	return dims
}

func (e *EKSDaemonTestRunner) getDimensionValue(dim string) string {
	switch dim {
	case "ClusterName":
		return e.env.EKSClusterName
	case "Namespace":
		return "amazon-cloudwatch"
	default:
		return ""
	}
}

func compareMaptoList(metricMap map[string]void, metricList []string) bool {
	if len(metricMap) != len(metricList) {
		return false
	}

	for _, key := range metricList {
		if _, ok := metricMap[key]; !ok {
			return false
		}
	}
	return true
}

func (e *EKSDaemonTestRunner) validateMetricData(name string, dims []types.Dimension) status.TestResult {
	log.Printf("validateMetricData with metric: %s", name)
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}
	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(containerInsightsNamespace, name, dims, metric.SAMPLE_COUNT, metric.HighResolutionStatPeriod)
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

func (e *EKSDaemonTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"cluster_failed_node_count",
		"cluster_node_count",
		"namespace_number_of_running_pods",
		"node_cpu_limit",
		"node_cpu_reserved_capacity",
		"node_cpu_usage_total",
		"node_cpu_utilization",
		"node_filesystem_utilization",
		"node_memory_limit",
		"node_memory_reserved_capacity",
		"node_memory_utilization",
		"node_memory_working_set",
		"node_network_total_bytes",
		"node_number_of_running_containers",
		"node_number_of_running_pods",
		"pod_cpu_reserved_capacity",
		"pod_cpu_utilization",
		"pod_cpu_utilization_over_pod_limit",
		"pod_memory_reserved_capacity",
		"pod_memory_utilization",
		"pod_memory_utilization_over_pod_limit",
		"pod_network_rx_bytes",
		"pod_network_tx_bytes",
		"service_number_of_running_pods",
	}
}

func (t *EKSDaemonTestRunner) SetAgentConfig(config test_runner.AgentConfig) {}

func (e *EKSDaemonTestRunner) SetupAfterAgentRun() error {
	return nil
}

var _ test_runner.ITestRunner = (*EKSDaemonTestRunner)(nil)
