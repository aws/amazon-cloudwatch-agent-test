// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// Must match JSON config
	statsdMetricsAggregationInterval = 30 * time.Second
	statsdMetricsCollectionInterval  = 5 * time.Second
	region                           = "us-west-2"
)

var (
	// metric name must use "_"" as a separator and include the metric type.
	StatsdMetricNames = []string{
		"statsd_counter_1",
		"statsd_gauge_2",
		"statsd_timing_3",
		"statsd_counter_4",
		"statsd_gauge_5",
		"statsd_timing_6",
	}
	StatsdMetricValues = []float64{1000, 2000, 3000, 4000, 5000, 6000}
)

func ValidateStatsdMetric(dimFactory dimension.Factory, namespace string, dimensionKey string, metricName string, expectedValue float64, runDuration time.Duration, sendInterval time.Duration) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	instructions := []dimension.Instruction{
		{
			Key:   dimensionKey,
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "key",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("value")},
		},
	}
	split := strings.Split(metricName, "_")
	if len(split) != 3 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
	instructions = append(instructions, dimension.Instruction{
		// CWA adds this metric_type dimension.
		Key:   "metric_type",
		Value: dimension.ExpectedDimensionValue{Value: aws.String(metricType)},
	})
	dims, failed := dimFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := MetricValueFetcher{}
	// Check average.
	values, err := fetcher.Fetch(namespace, metricName, dims, AVERAGE, HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	lowerBound := int(runDuration/statsdMetricsAggregationInterval) - 4
	if len(values) < lowerBound {
		log.Printf("fail: lowerBound %v, actual %v", lowerBound, len(values))
		return testResult
	}
	// Counters get summed up over the metrics_collection_interval.
	if metricType == "counter" {
		expectedValue *= float64(statsdMetricsCollectionInterval / sendInterval)
	}
	if !IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedValue)) {
		return testResult
	}
	// Check aggregation by checking sample count.
	// Expect samples to be metrics_aggregation_interval / metrics_collection_interval
	expectedSampleCount := statsdMetricsAggregationInterval / statsdMetricsCollectionInterval
	if metricType == "timing" {
		// Every single timing is counted.
		// Sent twice per send_interval.
		expectedSampleCount = 2 * statsdMetricsAggregationInterval / sendInterval
	}
	values, err = fetcher.Fetch(namespace, metricName, dims, SAMPLE_COUNT, HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}
	// Skip check on the last value.
	values = values[:len(values)-1]
	if !IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedSampleCount)) {
		return testResult
	}

	env := environment.GetEnvironmentMetaData()
	var computeType string

	switch env.ComputeType {
	case computetype.ECS:
		computeType = "ECS"
	case computetype.EKS:
		computeType = "EKS"
	default:
		computeType = "EC2"
	}

	err = ValidateStatsdEntity(metricName, metricType, computeType)
	if err != nil {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func GetExpectedEntity(computeType string) string {
	switch computeType {
	case "EC2":
		return `{"Entities":[{"__type":"com.amazonaws.observability#Entity","Attributes":{"AWS.ServiceNameSource":"ClientIamRole"},"KeyAttributes":{"Environment":"ec2:default","Type":"Service","Name":"cwa-e2e-iam-role"}}]}`
	case "EKS":
		return `{"Entities":[{"__type":"com.amazonaws.observability#Entity","Attributes":{"AWS.ServiceNameSource":"ServerIamRole"},"KeyAttributes":{"Environment":"ec2:default","Type":"Service","Name":"cwagent-eks-Worker-Role-01234567890"}}]}`
	case "ECS":
		return `{"Entities":[{"__type":"com.amazonaws.observability#Entity","Attributes":{"AWS.ServiceNameSource":"ServerIamRole"},"KeyAttributes":{"Environment":"ecs:cwagent-integ-test-cluster-01234567890","Type":"Service","Name":"cwa-e2e-iam-role"}}]}`
	}
}

func ValidateStatsdEntity(metricName, metricType, computeType string) error {
	// build the ListEntitiesForMetric request
	var requestBody []byte

	switch computeType {
	case "EC2":
		instanceId := awsservice.GetInstanceId()
		requestBody = []byte(fmt.Sprintf(`{
			"Namespace": "MetricValueBenchmarkTest",
			"MetricName": "%s",
			"Dimensions": [
				{
					"Name": "InstanceId",
					"Value": "%s"
				},
				{
					"Name": "key",
					"Value": "value"
				},
				{
					"Name": "metric_type",
					"Value": "%s"
				}
			]
		}`, metricName, instanceId, metricType))
	case "EKS":
		clusterName := awsservice.Cluster
		requestBody = []byte(fmt.Sprintf(`{
			"Namespace": "StatsD/EKS",
			"MetricName": "%s",
			"Dimensions": [
				{
					"Name": "ClusterName",
					"Value": "%s"
				},
				{
					"Name": "key",
					"Value": "value"
				},
				{
					"Name": "metric_type",
					"Value": "%s"
				}
			]
		}`, metricName, instanceId, metricType))
	case "ECS":
		instanceId := awsservice.GetInstanceId()
		requestBody = []byte(fmt.Sprintf(`{
			"Namespace": "StatsD/ECS",
			"MetricName": "%s",
			"Dimensions": [
				{
					"Name": "InstanceId",
					"Value": "%s"
				},
				{
					"Name": "key",
					"Value": "value"
				},
				{
					"Name": "metric_type",
					"Value": "%s"
				}
			]
		}`, metricName, instanceId, metricType))
	}

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, region)
	if err != nil {
		return fmt.Errorf("Error building the ListEntitiesForMetric request %v", err)
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error sending the ListEntitiesForMetric request %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading response body: %v", err)
	}

	if GetExpectedEntity(computeType) != string(responseBody) {
		fmt.Printf("Response Body: %s\n", string(responseBody))
		fmt.Printf("Expected Entity: %s\n", GetExpectedEntity(computeType))
		return fmt.Errorf("Response body doesn't match expected entity")
	}
	return nil
}
