// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package entity_metrics_benchmark

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type EntityMetricsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*EntityMetricsTestRunner)(nil)

type expectedEntity struct {
	entityType   string
	resourceType string
	instanceId   string
}

const (
	region = "us-west-2"
)

func (t *EntityMetricsTestRunner) Validate() status.TestGroupResult {
	instanceId := awsservice.GetInstanceId()

	testCases := map[string]struct {
		requestBody    []byte
		expectedEntity expectedEntity
	}{
		"ResourceMetrics/CPU": {
			requestBody: []byte(fmt.Sprintf(`{
                "Namespace": "CWAgent",
                "MetricName": "cpu_usage_idle",
                "Dimensions": [
                    {"Name": "InstanceId", "Value": "%s"},
                    {"Name": "cpu", "Value": "cpu-total"}
                ]
            }`, instanceId)),
			expectedEntity: expectedEntity{
				entityType:   "AWS::Resource",
				resourceType: "AWS::EC2::Instance",
				instanceId:   instanceId,
			},
		},
	}

	var testResults []status.TestResult

	for name, testCase := range testCases {
		testResult := t.validateTestCase(name, testCase)
		testResults = append(testResults, testResult)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EntityMetricsTestRunner) validateTestCase(name string, testCase struct {
	requestBody    []byte
	expectedEntity expectedEntity
}) status.TestResult {
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}

	req, err := common.BuildListEntitiesForMetricRequest(testCase.requestBody, region)
	if err != nil {
		log.Printf("Failed to build ListEntitiesForMetric request for test case '%s': %v", name, err)
		return testResult
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send request for test case '%s': %v", name, err)
		return testResult
	}
	defer resp.Body.Close()

	// parse and verify the response
	var response struct {
		Entities []struct {
			KeyAttributes struct {
				Type         string `json:"Type"`
				ResourceType string `json:"ResourceType"`
				Identifier   string `json:"Identifier"`
			} `json:"KeyAttributes"`
		} `json:"Entities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Failed to decode response for test case '%s': %v", name, err)
		return testResult
	}

	if len(response.Entities) == 0 {
		log.Printf("Response contains no entities for test case '%s'", name)
		return testResult
	}

	entity := response.Entities[0]
	if entity.KeyAttributes.Type != testCase.expectedEntity.entityType ||
		entity.KeyAttributes.ResourceType != testCase.expectedEntity.resourceType ||
		entity.KeyAttributes.Identifier != testCase.expectedEntity.instanceId {

		log.Printf("Entity mismatch for test case '%s':\n"+
			"Expected:\n"+
			"  Type: %s\n"+
			"  ResourceType: %s\n"+
			"  InstanceId: %s\n"+
			"Got:\n"+
			"  Type: %s\n"+
			"  ResourceType: %s\n"+
			"  InstanceId: %s",
			name,
			testCase.expectedEntity.entityType,
			testCase.expectedEntity.resourceType,
			testCase.expectedEntity.instanceId,
			entity.KeyAttributes.Type,
			entity.KeyAttributes.ResourceType,
			entity.KeyAttributes.Identifier)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *EntityMetricsTestRunner) GetTestName() string {
	return "EntityMetrics"
}

func (t *EntityMetricsTestRunner) GetAgentConfigFileName() string {
	return "entity_metrics.json"
}

func (t *EntityMetricsTestRunner) GetMeasuredMetrics() []string {
	return []string{"cpu-total"}
}

func (t *EntityMetricsTestRunner) GetAgentRunDuration() time.Duration {
	return 4 * time.Minute
}
