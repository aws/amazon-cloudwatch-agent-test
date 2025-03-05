// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

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

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type CollectDTestRunner struct {
	test_runner.BaseTestRunner
}
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

var _ test_runner.ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch)*2)
	for i, metricName := range metricsToFetch {
		// First test result is for metric validation
		testResults[i*2] = t.validateCollectDMetric(metricName)
		// Second test result is for the entity validation associated with the metric
		testResults[i*2+1] = t.validateCollectDEntity(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDTestRunner) GetTestName() string {
	return "CollectD"
}

func (t *CollectDTestRunner) GetAgentConfigFileName() string {
	return "collectd_config.json"
}

func (t *CollectDTestRunner) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectDTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value", "collectd_counter_1_value"}
}

func (t *CollectDTestRunner) GetExpectedEntity() []Entity {
	return []Entity{
		{
			Type: "com.amazonaws.observability#Entity",
			KeyAttributes: KeyAttributes{
				Environment: "ec2:default",
				Type:        "Service",
				Name:        "cwa-e2e-iam-role",
			},
		},
	}
}

func (t *CollectDTestRunner) validateCollectDMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	instructions := []dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	}

	switch metricName {
	case "collectd_counter_1_value":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("counter")},
		})
	case "collectd_gauge_1_value":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("gauge")},
		})
	}

	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	if err != nil {
		return testResult
	}

	runDuration := t.GetAgentRunDuration()
	aggregationInterval := 30 * time.Second
	// If aggregation is not happening there could be a data point every 5 seconds.
	// So validate the upper bound.
	upperBound := int(runDuration/aggregationInterval) + 2
	// Allow 2 missing data points in case CW-Metrics-Web-Service has a 1 minute
	// delay to store.
	lowerBound := int(runDuration/aggregationInterval) - 2

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 1) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *CollectDTestRunner) validateCollectDEntity(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("%s_entity", metricName),
		Status: status.FAILED,
	}
	// build the ListEntitiesForMetric request
	metricType := t.GetMetricType(metricName)
	instanceId := awsservice.GetInstanceId()

	requestBody := []byte(fmt.Sprintf(`{
		"Namespace": "MetricValueBenchmarkTest",
		"MetricName": "%s",
		"Dimensions": [
			{
				"Name": "InstanceId",
				"Value": "%s"
			},
			{
				"Name": "type",
				"Value": "%s"
			}
		]
	}`, metricName, instanceId, metricType))

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, "us-west-2")
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

	expectedEntity := t.GetExpectedEntity()

	if !reflect.DeepEqual(expectedEntity, actualEntities.Entities) {
		log.Printf("Actual entity doesn't match expected entity\nActual Entity: %+v\nExpected Entity: %+v\n",
			actualEntities, expectedEntity)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *CollectDTestRunner) GetAgentRunDuration() time.Duration {
	return time.Minute
}

func (t *CollectDTestRunner) GetMetricType(metricName string) string {
	split := strings.Split(metricName, "_")
	if len(split) != 4 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
	return metricType
}
