// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const containerInsightsNamespace = "ContainerInsights"
const gpuMetricIndicator = "_gpu_"

// list of metrics with more dimensions e.g. PodName and Namespace
var metricsWithMoreDimensions = []string{"pod_number_of_container_restarts"}

var expectedDimsToMetrics = map[string][]string{
	"ClusterName": {
		"pod_number_of_containers",
		"node_status_allocatable_pods",
		"pod_number_of_container_restarts",
		"node_status_condition_unknown",
		"node_number_of_running_pods",
		"pod_container_status_running",
		"node_status_condition_ready",
		"pod_status_running",
		"node_filesystem_utilization",
		"pod_container_status_terminated",
		"pod_status_pending",
		"pod_cpu_utilization",
		"node_filesystem_inodes",
		"node_diskio_io_service_bytes_total",
		"node_status_condition_memory_pressure",
		"container_cpu_utilization",
		"service_number_of_running_pods",
		"pod_memory_utilization_over_pod_limit",
		"node_memory_limit",
		"pod_cpu_request",
		"pod_interface_network_tx_dropped",
		"pod_status_succeeded",
		"namespace_number_of_running_pods",
		"pod_memory_reserved_capacity",
		"node_diskio_io_serviced_total",
		"pod_network_rx_bytes",
		"node_status_capacity_pods",
		"pod_status_unknown",
		"cluster_failed_node_count",
		"container_memory_utilization",
		"node_memory_utilization",
		"node_filesystem_inodes_free",
		"container_memory_request",
		"container_cpu_limit",
		"node_memory_reserved_capacity",
		"node_interface_network_tx_dropped",
		"pod_cpu_utilization_over_pod_limit",
		"container_memory_failures_total",
		"pod_status_ready",
		"pod_number_of_running_containers",
		"cluster_node_count",
		"pod_memory_request",
		"node_cpu_utilization",
		"cluster_number_of_running_pods",
		"node_memory_working_set",
		"pod_status_failed",
		"node_status_condition_pid_pressure",
		"pod_status_scheduled",
		"node_number_of_running_containers",
		"node_cpu_limit",
		"node_status_condition_disk_pressure",
		"pod_cpu_limit",
		"pod_memory_limit",
		"node_cpu_usage_total",
		"pod_cpu_reserved_capacity",
		"pod_network_tx_bytes",
		"container_memory_limit",
		"pod_memory_utilization",
		"node_interface_network_rx_dropped",
		"node_network_total_bytes",
		"container_cpu_utilization_over_container_limit",
		"pod_interface_network_rx_dropped",
		"pod_container_status_waiting",
		"node_cpu_reserved_capacity",
		"container_memory_utilization_over_container_limit",
		"container_cpu_request",
	},
	"ClusterName-FullPodName-Namespace-PodName": {
		"pod_network_tx_bytes",
		"pod_interface_network_rx_dropped",
		"pod_cpu_limit",
		"pod_status_succeeded",
		"pod_container_status_waiting",
		"pod_number_of_running_containers",
		"pod_status_succeeded",
		"pod_number_of_running_containers",
		"pod_number_of_container_restarts",
		"pod_status_pending",
		"pod_status_running",
		"pod_container_status_running",
		"pod_container_status_running",
		"pod_interface_network_rx_dropped",
		"pod_memory_limit",
		"pod_status_unknown",
		"pod_memory_utilization_over_pod_limit",
		"pod_container_status_waiting",
		"pod_cpu_request",
		"pod_status_unknown",
		"pod_status_scheduled",
		"pod_memory_utilization",
		"pod_container_status_waiting",
		"pod_container_status_waiting",
		"pod_status_failed",
		"pod_number_of_container_restarts",
		"pod_network_rx_bytes",
		"pod_network_rx_bytes",
		"pod_number_of_containers",
		"pod_status_succeeded",
		"pod_memory_utilization",
		"pod_cpu_utilization",
		"pod_memory_reserved_capacity",
		"pod_memory_utilization_over_pod_limit",
		"pod_number_of_containers",
		"pod_memory_utilization_over_pod_limit",
		"pod_status_ready",
		"pod_status_pending",
		"pod_number_of_containers",
		"pod_container_status_waiting",
		"pod_container_status_terminated",
		"pod_status_running",
		"pod_status_pending",
		"pod_container_status_terminated",
		"pod_interface_network_tx_dropped",
		"pod_status_running",
		"pod_network_rx_bytes",
		"pod_interface_network_tx_dropped",
		"pod_cpu_request",
		"pod_status_unknown",
		"pod_memory_request",
		"pod_container_status_terminated",
		"pod_cpu_utilization",
		"pod_number_of_running_containers",
		"pod_memory_reserved_capacity",
		"pod_status_failed",
		"pod_interface_network_rx_dropped",
		"pod_status_ready",
		"pod_cpu_reserved_capacity",
		"pod_status_unknown",
		"pod_network_tx_bytes",
		"pod_cpu_request",
		"pod_status_failed",
		"pod_status_succeeded",
		"pod_cpu_utilization_over_pod_limit",
		"pod_status_ready",
		"pod_memory_utilization",
		"pod_status_running",
		"pod_status_failed",
		"pod_memory_limit",
		"pod_cpu_utilization",
		"pod_memory_limit",
		"pod_number_of_container_restarts",
		"pod_status_ready",
		"pod_status_scheduled",
		"pod_container_status_terminated",
		"pod_cpu_reserved_capacity",
		"pod_container_status_running",
		"pod_number_of_container_restarts",
		"pod_cpu_reserved_capacity",
		"pod_status_pending",
		"pod_status_ready",
		"pod_status_failed",
		"pod_status_unknown",
		"pod_container_status_running",
		"pod_memory_request",
		"pod_container_status_running",
		"pod_status_pending",
		"pod_memory_utilization",
		"pod_network_rx_bytes",
		"pod_number_of_container_restarts",
		"pod_status_scheduled",
		"pod_cpu_request",
		"pod_memory_reserved_capacity",
		"pod_container_status_terminated",
		"pod_cpu_reserved_capacity",
		"pod_network_tx_bytes",
		"pod_status_scheduled",
		"pod_network_rx_bytes",
		"pod_number_of_containers",
	},
	"ClusterName-Namespace-PodName":{
		"pod_interface_network_rx_dropped",
		"pod_status_succeeded",
		"pod_container_status_running",
		"pod_network_rx_bytes",
		"pod_cpu_utilization",
		"pod_memory_utilization",
		"pod_interface_network_tx_dropped",
		"pod_status_ready",
		"pod_status_ready",
		"pod_container_status_terminated",
		"pod_cpu_reserved_capacity",
		"pod_memory_request",
		"pod_status_running",
		"pod_interface_network_rx_dropped",
		"pod_status_succeeded",
		"pod_status_pending",
		"pod_number_of_containers",
		"pod_status_succeeded",
		"pod_status_pending",
		"pod_memory_utilization_over_pod_limit",
		"pod_status_unknown",
		"pod_cpu_limit",
		"pod_container_status_waiting",
		"pod_memory_request",
		"pod_status_running",
		"pod_memory_reserved_capacity",
		"pod_cpu_reserved_capacity",
		"pod_network_tx_bytes",
		"pod_cpu_utilization",
		"pod_status_failed",
		"pod_network_tx_bytes",
		"pod_number_of_running_containers",
		"pod_memory_reserved_capacity",
		"pod_number_of_running_containers",
		"pod_status_failed",
		"pod_status_unknown",
		"pod_number_of_containers",
		"pod_container_status_terminated",
		"pod_status_failed",
		"pod_status_unknown",
		"pod_number_of_container_restarts",
		"pod_container_status_waiting",
		"pod_cpu_reserved_capacity",
		"pod_cpu_request",
		"pod_interface_network_tx_dropped",
		"pod_cpu_utilization_over_pod_limit",
		"pod_number_of_running_containers",
		"pod_status_unknown",
		"pod_network_rx_bytes",
		"pod_cpu_request",
		"pod_cpu_request",
		"pod_number_of_container_restarts",
		"pod_container_status_waiting",
		"pod_number_of_running_containers",
		"pod_status_ready",
		"pod_number_of_containers",
		"pod_interface_network_tx_dropped",
		"pod_interface_network_tx_dropped",
		"pod_status_scheduled",
		"pod_memory_limit",
		"pod_memory_limit",
		"pod_memory_utilization_over_pod_limit",
		"pod_container_status_running",
		"pod_status_scheduled",
		"pod_number_of_containers",
		"pod_status_pending",
		"pod_memory_utilization",
		"pod_network_rx_bytes",
		"pod_interface_network_rx_dropped",
		"pod_number_of_container_restarts",
		"pod_status_ready",
		"pod_container_status_running",
		"pod_memory_utilization",
		"pod_cpu_utilization",
		"pod_status_running",
		"pod_status_running",
		"pod_status_scheduled",
		"pod_status_pending",
		"pod_status_succeeded",
		"pod_network_tx_bytes",
		"pod_status_failed",
		"pod_container_status_terminated",
		"pod_status_scheduled",
		"pod_container_status_terminated",
		"pod_network_tx_bytes",
		"pod_interface_network_rx_dropped",
		"pod_container_status_waiting",
		"pod_cpu_utilization",
		"pod_cpu_request",
		"pod_number_of_container_restarts",
		"pod_memory_utilization",
		"pod_network_rx_bytes",
		"pod_cpu_reserved_capacity",
		"pod_container_status_running",

	},
	"ClusterName-ContainerName-FullPodName-Namespace":{
		"container_memory_failures_total",
		"container_cpu_utilization",
		"container_cpu_utilization_over_container_limit",
		"container_memory_limit",
		"container_memory_failures_total",
		"container_memory_utilization_over_container_limit",
		"container_memory_utilization_over_container_limit",
		"container_memory_utilization",
		"container_memory_failures_total",
		"container_cpu_request",
		"container_cpu_utilization",
		"container_memory_failures_total",
		"container_memory_request",
		"container_memory_request",
		"container_memory_utilization",
		"container_memory_request",
		"container_cpu_request",
		"container_memory_limit",
		"container_memory_utilization",
		"container_cpu_utilization",
		"container_cpu_request",
		"container_memory_limit",
		"container_memory_utilization",
		"container_cpu_utilization",
		"container_cpu_utilization",
		"container_memory_failures_total",
		"container_memory_utilization",
		"container_cpu_utilization",
		"container_cpu_request",
		"container_cpu_limit",
		"container_memory_utilization_over_container_limit",
		"container_cpu_request",
		"container_memory_failures_total",
		"container_cpu_request",
		"container_memory_utilization",
	},
	"ClusterName-ContainerName-NameSpace-PodName":{
		"container_memory_request",
		"container_memory_utilization_over_container_limit",
		"container_cpu_limit",
		"container_memory_failures_total",
		"container_memory_utilization",
		"container_memory_failures_total",
		"container_cpu_utilization",
		"container_memory_utilization",
		"container_memory_failures_total",
		"container_cpu_utilization_over_container_limit",
		"container_cpu_request",
		"container_memory_utilization",
		"container_memory_utilization",
		"container_cpu_utilization",
		"container_memory_failures_total",
		"container_memory_request",
		"container_cpu_request",
		"container_cpu_request",
		"container_cpu_request",
		"container_memory_limit",
		"container_memory_utilization_over_container_limit",
		"container_memory_limit",
		"container_cpu_utilization",
		"container_memory_utilization",
		"container_cpu_utilization",
		"container_cpu_request",
		"container_memory_failures_total",
		"container_cpu_utilization",
	},
	"ClusterName-InstanceId-NodeName":{
		"node_status_allocatable_pods",
		"node_network_total_bytes",
		"node_status_condition_unknown",
		"node_interface_network_rx_dropped",
		"node_number_of_running_containers",
		"node_interface_network_tx_dropped",
		"node_memory_utilization",
		"node_cpu_limit",
		"node_status_condition_disk_pressure",
		"node_memory_working_set",
		"node_cpu_reserved_capacity",
		"node_status_condition_ready",
		"node_filesystem_utilization",
		"node_status_condition_memory_pressure",
		"node_memory_limit",
		"node_memory_reserved_capacity",
		"node_diskio_io_serviced_total",
		"node_status_condition_pid_pressure",
		"node_filesystem_inodes",
		"node_cpu_usage_total",
		"node_number_of_running_pods",
		"node_diskio_io_service_bytes_total",
		"node_status_capacity_pods",
		"node_filesystem_inodes_free",
		"node_cpu_utilization",

	},

	"ClusterName-Namespace-Service":{
		"pod_status_unknown",
		"pod_memory_limit",
		"pod_container_status_terminated",
		"pod_status_ready",
		"pod_number_of_container_restarts",
		"pod_status_pending",
		"pod_status_succeeded",
		"pod_network_rx_bytes",
		"pod_status_failed",
		"pod_number_of_containers",
		"pod_cpu_request",
		"service_number_of_running_pods",
		"pod_memory_reserved_capacity",
		"pod_network_tx_bytes",
		"pod_container_status_waiting",
		"pod_memory_request",
		"pod_status_running",
		"pod_container_status_running",
		"pod_cpu_reserved_capacity",
		"pod_memory_utilization_over_pod_limit",
		"pod_cpu_utilization",
		"pod_memory_utilization",
		"pod_number_of_running_containers",
		"pod_status_scheduled",
	},
	"ClusterName-Namespace"{
		"pod_interface_network_rx_dropped",
		"pod_network_rx_bytes",
		"pod_cpu_utilization_over_pod_limit",
		"pod_memory_utilization_over_pod_limit",
		"namespace_number_of_running_pods",
		"pod_memory_utilization",
		"pod_memory_utilization",
		"pod_interface_network_tx_dropped",
		"pod_cpu_utilization",
		"pod_interface_network_tx_dropped",
		"namespace_number_of_running_pods",
		"pod_network_tx_bytes",
		"pod_interface_network_rx_dropped",
		"pod_network_tx_bytes",
		"pod_memory_utilization_over_pod_limit",
		"pod_network_rx_bytes",
		"pod_cpu_utilization",
	},
}

type EKSDaemonTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

func (e *EKSDaemonTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, ValidateMetrics(e.env, gpuMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, e.validateLogs(e.env))
	return status.TestGroupResult{
		Name:        e.GetTestName(),
		TestResults: testResults,
	}
}

type void struct{}

const (
	dimDelimiter               = "-"
	ContainerInsightsNamespace = "ContainerInsights"
)

type dimToMetrics struct {
	// dim keys as string with dimDelimiter(-) eg. ClusterName-Namespace
	dimStr string
	// metric names to their dimensions with values. Dimension sets will be used for metric data validations
	metrics map[string][][]types.Dimension
}

func ValidateMetrics(env *environment.MetaData, metricFilter string, expectedDimsToMetrics map[string][]string) []status.TestResult {
	var results []status.TestResult
	dimsToMetrics := getMetricsInClusterDimension(env, metricFilter)
	for dims, metrics := range expectedDimsToMetrics {
		var actual map[string][][]types.Dimension
		for _, dtm := range dimsToMetrics {
			if dtm.dimStr == dims {
				actual = dtm.metrics
				break
			}
		}
		if len(actual) < 1 {
			results = append(results, status.TestResult{
				Name:   dims,
				Status: status.FAILED,
			})
			log.Printf("ValidateMetrics failed with missing dimension set: %s", dims)
			// keep testing other dims or fail early?
			continue
		}
		results = append(results, validateMetricsAvailability(dims, metrics, actual))
		for _, m := range metrics {
			// picking a random dimension set to test metric data so we don't have to test every dimension set
			randIdx := rand.Intn(len(actual[m]))
			results = append(results, validateMetricValue(m, actual[m][randIdx]))
		}
	}
	return results
}

func getMetricsInClusterDimension(env *environment.MetaData, metricFilter string) []dimToMetrics { //map[string]map[string]interface{} {
	listFetcher := metric.MetricListFetcher{}
	log.Printf("Fetching by cluster dimension")
	dims := []types.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(env.EKSClusterName),
		},
	}
	metrics, err := listFetcher.Fetch(ContainerInsightsNamespace, "", dims)
	if err != nil {
		log.Println("failed to fetch metric list", err)
		return nil
	}
	if len(metrics) < 1 {
		log.Println("cloudwatch metric list is empty")
		return nil
	}

	var results []dimToMetrics
	for _, m := range metrics {
		// filter by metric name filter(skip gpu validation)
		if metricFilter != "" && strings.Contains(*m.MetricName, metricFilter) {
			continue
		}
		var dims []string
		for _, d := range m.Dimensions {
			dims = append(dims, *d.Name)
		}
		sort.Sort(sort.StringSlice(dims)) //what's the point of sorting?
		dimsKey := strings.Join(dims, dimDelimiter)
		log.Printf("processing dims: %s", dimsKey)

		var dtm dimToMetrics
		for _, ele := range results {
			if ele.dimStr == dimsKey {
				dtm = ele
				break
			}
		}
		if dtm.dimStr == "" {
			dtm = dimToMetrics{
				dimStr:  dimsKey,
				metrics: make(map[string][][]types.Dimension),
			}
			results = append(results, dtm)
		}
		dtm.metrics[*m.MetricName] = append(dtm.metrics[*m.MetricName], m.Dimensions)
	}
	return results
}

func validateMetricsAvailability(dims string, expected []string, actual map[string][][]types.Dimension) status.TestResult {
	testResult := status.TestResult{
		Name:   dims,
		Status: status.FAILED,
	}
	log.Printf("expected metrics: %d, actual metrics: %d", len(expected), len(actual))
	if compareMetrics(expected, actual) {
		testResult.Status = status.SUCCESSFUL
	} else {
		log.Printf("validateMetricsAvailability failed for %s", dims)
	}
	return testResult
}

func compareMetrics(expected []string, actual map[string][][]types.Dimension) bool {
	if len(expected) != len(actual) {
		return false
	}

	for _, key := range expected {
		if _, ok := actual[key]; !ok {
			return false
		}
	}
	return true
}

func validateMetricValue(name string, dims []types.Dimension) status.TestResult {
	log.Printf("validateMetricValue with metric: %s", name)
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}
	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(containerInsightsNamespace, name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
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
