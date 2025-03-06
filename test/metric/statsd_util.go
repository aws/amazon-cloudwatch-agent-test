// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
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
	EKSNamespace                     = "StatsD/EKS"
	ECSNamespace                     = "StatsD/ECS"
	EC2Namespace                     = "MetricValueBenchmarkTest"
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

type Entity struct {
	Type          string        `json:"__type"`
	Attributes    Attributes    `json:"Attributes"`
	KeyAttributes KeyAttributes `json:"KeyAttributes"`
}

type Attributes struct {
	ServiceNameSource string `json:"AWS.ServiceNameSource"`
}

type KeyAttributes struct {
	Environment string `json:"Environment"`
	Type        string `json:"Type"`
	Name        string `json:"Name"`
}
type Dimension struct {
	Name  string
	Value string
}

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
	metricType := GetMetricType(metricName)
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

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func ValidateStatsdEntity(dimFactory dimension.Factory, metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("%s_entity", metricName),
		Status: status.FAILED,
	}

	env := environment.GetEnvironmentMetaData()
	metricType := GetMetricType(metricName)

	// build the ListEntitiesForMetric request
	var dimensions []Dimension
	var namespace string

	switch env.ComputeType {
	case computetype.EC2:
		dimensions = []Dimension{
			{Name: "InstanceId", Value: awsservice.GetInstanceId()},
			{Name: "key", Value: "value"},
			{Name: "metric_type", Value: metricType},
		}
		namespace = EC2Namespace
	case computetype.EKS:
		dimensions = []Dimension{
			{Name: "ClusterName", Value: env.EKSClusterName},
			{Name: "key", Value: "value"},
			{Name: "metric_type", Value: metricType},
		}
		namespace = EKSNamespace
	case computetype.ECS:
		instructions := []dimension.Instruction{
			{
				Key:   "InstanceId",
				Value: dimension.UnknownDimensionValue(),
			},
		}
		dims, failed := dimFactory.GetDimensions(instructions)
		if len(failed) > 0 {
			return testResult
		}

		instanceId := *dims[0].Value
		dimensions = []Dimension{
			{Name: "InstanceId", Value: instanceId},
			{Name: "key", Value: "value"},
			{Name: "metric_type", Value: metricType},
		}
		namespace = ECSNamespace
	default:
		return testResult
	}

	requestBody, err := buildRequestBody(namespace, metricName, dimensions)
	if err != nil {
		return testResult
	}

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, region)
	if err != nil {
		log.Printf("Error building the ListEntitiesForMetric request %v", err)
		return testResult
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending the ListEntitiesForMetric request %v", err)
		return testResult
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return testResult
	}

	var actualEntities struct {
		Entities []Entity `json:"Entities"`
	}

	if err := json.Unmarshal(responseBody, &actualEntities); err != nil {
		log.Printf("Error unmarshaling response body: %v", err)
		return testResult
	}

	expectedEntity, err := GetExpectedEntity(string(env.ComputeType))

	if err != nil {
		log.Printf("Error getting the expected entity: %v", err)
		return testResult
	}

	if !reflect.DeepEqual(expectedEntity, actualEntities.Entities) {
		log.Printf("Actual entity doesn't match expected entity\nActual Entity: %+v\nExpected Entity: %+v\n", actualEntities, expectedEntity)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func buildRequestBody(namespace, metricName string, dimensions []Dimension) ([]byte, error) {
	request := struct {
		Namespace  string      `json:"Namespace"`
		MetricName string      `json:"MetricName"`
		Dimensions []Dimension `json:"Dimensions"`
	}{
		Namespace:  namespace,
		MetricName: metricName,
		Dimensions: dimensions,
	}

	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

func GetExpectedEntity(computeType string) ([]Entity, error) {
	env := environment.GetEnvironmentMetaData()

	// Base entity that's common across all types
	entity := Entity{
		Type: "com.amazonaws.observability#Entity",
		KeyAttributes: KeyAttributes{
			Type: "Service",
		},
	}

	switch computeType {
	case "EC2":
		entity.Attributes = Attributes{
			ServiceNameSource: "ClientIamRole",
		}
		entity.KeyAttributes.Environment = "ec2:default"
		entity.KeyAttributes.Name = "cwa-e2e-iam-role"

	case "EKS":
		name := strings.Replace(env.EKSClusterName, "cwagent-eks-integ", "cwagent-eks-Worker-Role", -1)
		entity.Attributes = Attributes{
			ServiceNameSource: "ServerIamRole",
		}
		entity.KeyAttributes.Environment = "ec2:default"
		entity.KeyAttributes.Name = name

	case "ECS":
		entity.Attributes = Attributes{
			ServiceNameSource: "ServerIamRole",
		}
		entity.KeyAttributes.Environment = fmt.Sprintf("ecs:%s", env.EcsClusterName)
		entity.KeyAttributes.Name = "cwa-e2e-iam-role"

	default:
		return nil, fmt.Errorf("Unexpected compute type while fetching the expected entity")
	}

	return []Entity{entity}, nil
}

func GetMetricType(metricName string) string {
	split := strings.Split(metricName, "_")
	if len(split) != 3 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
	return metricType
}
