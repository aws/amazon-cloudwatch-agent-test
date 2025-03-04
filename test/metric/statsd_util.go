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
	// identifier is what is needed for the ListEntitiesForMetric API call
	// instance id (EC2 and ECS) or cluster name (EKS)
	var identifier string

	switch env.ComputeType {
	case computetype.EC2:
		identifier = awsservice.GetInstanceId()
	case computetype.EKS:
		identifier = env.EKSClusterName
	case computetype.ECS:
		for _, dim := range dims {
			if *dim.Name == "InstanceId" {
				identifier = *dim.Value
				break
			}
		}
	default:
		return testResult
	}

	if err := ValidateStatsdEntity(metricName, metricType, string(env.ComputeType), identifier); err != nil {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func ValidateStatsdEntity(metricName, metricType, computeType, identifier string) error {
	// build the ListEntitiesForMetric request
	var requestBody []byte

	switch computeType {
	case "EC2":
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
		}`, metricName, identifier, metricType))
	case "EKS":
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
		}`, metricName, identifier, metricType))
	case "ECS":
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
		}`, metricName, identifier, metricType))
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

	expectedEntity, err := GetExpectedEntity(computeType)
	if err != nil {
		return fmt.Errorf("Error getting the expected entity: %v", err)
	}

	var actualEntities []Entity
	if err := json.Unmarshal(responseBody, &actualEntities); err != nil {
		return fmt.Errorf("Error unmarshaling response body: %v", err)
	}

	if !reflect.DeepEqual(expectedEntity, actualEntities) {
		return fmt.Errorf("Actual entity doesn't match expected entity\nActual Entity: %+v\nExpected Entity: %+v\n",
			actualEntities, expectedEntity)
	}

	return nil
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
