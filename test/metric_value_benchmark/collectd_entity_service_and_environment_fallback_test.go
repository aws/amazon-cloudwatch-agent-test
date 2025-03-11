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
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type CollectDEntityFallback struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectDEntityFallback)(nil)

func (t *CollectDEntityFallback) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))

	for i, metricName := range metricsToFetch {
		testResults[i] = t.ValidateCollectDEntity(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDEntityFallback) GetTestName() string {
	return "CollectDEntity - Service Name and Environment Fallback"
}

func (t *CollectDEntityFallback) GetAgentConfigFileName() string {
	return "collectd_entity_service_and_environment_fallback_config.json"
}

func (t *CollectDEntityFallback) SetupAfterAgentRun() error {
	return common.SendCollectDMetrics(2, time.Second, t.GetAgentRunDuration())
}

func (t *CollectDEntityFallback) GetMeasuredMetrics() []string {
	return []string{"collectd_counter_1_value"}
}

func (t *CollectDEntityFallback) ValidateCollectDEntity(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("%s_entity", metricName),
		Status: status.FAILED,
	}

	requestBody, err := metric.BuildRequestBody(metricNamespace, metricName)
	if err != nil {
		log.Printf("Error building the body for the ListEntitiesForMetric request %v", err)
	}

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, "us-west-2")
	if err != nil {
		log.Printf("Error building the complete ListEntitiesForMetric request %v", err)
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
		Entities []metric.Entity `json:"Entities"`
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

func (t *CollectDEntityFallback) GetAgentRunDuration() time.Duration {
	return time.Minute
}

func (t *CollectDEntityFallback) GetExpectedEntity() []metric.Entity {
	return []metric.Entity{
		{
			Type: "com.amazonaws.observability#Entity",
			KeyAttributes: metric.KeyAttributes{
				Environment: "ec2:default",
				Type:        "Service",
				Name:        "cwa-e2e-iam-role",
			},
		},
	}
}
