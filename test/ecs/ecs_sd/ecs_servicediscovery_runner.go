// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ecs_sd

import (
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	MaxRetryCount = 15
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
)

// ServiceDiscoveryScenario represents a specific ECS service discovery test scenario
type ServiceDiscoveryScenario struct {
	Name                string
	ConfigFile          string
	LogStreamName       string
	ExpectedJobName     string
	ExpectedServiceType string
}

var (
	// Define the test scenarios
	serviceDiscoveryScenarios = []ServiceDiscoveryScenario{
		{
			Name:                "DockerLabel",
			ConfigFile:          "config.json",
			LogStreamName:       "prometheus-redis",
			ExpectedJobName:     "prometheus-redis",
			ExpectedServiceType: "docker_label",
		},
		{
			Name:                "TaskDefinitionList",
			ConfigFile:          "config_task_definition_list.json",
			LogStreamName:       "task-def-redis",
			ExpectedJobName:     "task-def-redis",
			ExpectedServiceType: "task_definition_list",
		},
		{
			Name:                "ServiceNameList",
			ConfigFile:          "config_service_name_list.json",
			LogStreamName:       "service-redis",
			ExpectedJobName:     "service-redis",
			ExpectedServiceType: "service_name_list_for_tasks",
		},
	}
)

//go:embed resources/emf_prometheus_redis_schema.json
var schema string

type ECSServiceDiscoveryTestRunner struct {
	test_runner.BaseTestRunner
}

func (t ECSServiceDiscoveryTestRunner) GetTestName() string {
	return "ecs_servicediscovery"
}

func (t ECSServiceDiscoveryTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t ECSServiceDiscoveryTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}

func (t ECSServiceDiscoveryTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	
	// Run validation for each scenario
	for _, scenario := range serviceDiscoveryScenarios {
		testResults = append(testResults, t.ValidateCloudWatchLogs(scenario))
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t ECSServiceDiscoveryTestRunner) ValidateCloudWatchLogs(scenario ServiceDiscoveryScenario) status.TestResult {
	env := environment.GetEnvironmentMetaData()
	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, env.EcsClusterName)

	testResult := status.TestResult{
		Name:   fmt.Sprintf("Retrieve Test LogGroup for %s scenario: %s", scenario.Name, logGroupName),
		Status: status.FAILED,
	}

	logGroupFound, err := t.ValidateLogGroupFormat(logGroupName, scenario)

	if logGroupFound {
		if err != nil {
			log.Printf("ECS ServiceDiscovery Test LogGroups invalid for %s scenario\n", scenario.Name)
			testResult.Name = err.Error()
			testResult.Status = status.FAILED
		} else {
			testResult.Status = status.SUCCESSFUL
		}
		awsservice.DeleteLogGroupAndStream(logGroupName, scenario.LogStreamName)
	}
	return testResult
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogGroupFormat(logGroupName string, scenario ServiceDiscoveryScenario) (bool, error) {
	start := time.Now()

	for retries := 0; retries < MaxRetryCount; retries++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			end := time.Now()
			return true, t.ValidateLogsContent(logGroupName, start, end, scenario)
		}

		log.Printf("Retry %d/%d: Log group not found for %s scenario. Waiting 20 seconds...\n", retries+1, MaxRetryCount, scenario.Name)
		time.Sleep(20 * time.Second)
	}

	log.Printf("ECS ServiceDiscovery Test has exhausted %v retry times for %s scenario", MaxRetryCount, scenario.Name)
	return false, fmt.Errorf("Test Retries Exhausted for %s scenario: %d", scenario.Name, MaxRetryCount)
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogsContent(logGroupName string, start time.Time, end time.Time, scenario ServiceDiscoveryScenario) error {
	return awsservice.ValidateLogs(
		logGroupName,
		scenario.LogStreamName,
		&start,
		&end,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogSchema(awsservice.WithSchema(schema)),
			func(event types.OutputLogEvent) error {
				if strings.Contains(*event.Message, "CloudWatchMetrics") &&
					!strings.Contains(*event.Message, "\"Namespace\":\"ECS/ContainerInsights/Prometheus\"") {
					return fmt.Errorf("emf log found for non ECS/ContainerInsights/Prometheus namespace: %s", *event.Message)
				}
				return nil
			},
			awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"job\":\"%s\"", scenario.ExpectedJobName)),
		),
	)
}
