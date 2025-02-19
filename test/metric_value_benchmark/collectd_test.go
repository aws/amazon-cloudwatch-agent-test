// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"fmt"
	"io"
	"log"
	"net/http"
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

var _ test_runner.ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectDMetric(metricName)
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

func (t *CollectDTestRunner) GetExpectedEntity() string {
	return `{"Entities":[{"__type":"com.amazonaws.observability#Entity","KeyAttributes":{"Environment":"ec2:default","Type":"Service","Name":"cwa-e2e-iam-role"}}]}`
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
	split := strings.Split(metricName, "_")
	if len(split) != 4 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
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

	err = t.ValidateCollectDEntity(metricName, metricType)
	if err != nil {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *CollectDTestRunner) ValidateCollectDEntity(metricName, metricType string) error {
	// build request
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
		return fmt.Errorf("Error building the ListEntitiesForMetric request %v", err)
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error sending the ListEntitiesForMetric request %v", err)
	}
	defer resp.Body.Close()

	// Read and print response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading response body: %v", err)
	}

	if t.GetExpectedEntity() != string(responseBody) {
		fmt.Printf("Response Body: %s\n", string(responseBody))
		fmt.Printf("Expected Entity: %s\n", t.GetExpectedEntity())
		return fmt.Errorf("Response body doesn't match expected entity")
	}

	return nil
}

func (t *CollectDTestRunner) GetAgentRunDuration() time.Duration {
	return time.Minute
}
