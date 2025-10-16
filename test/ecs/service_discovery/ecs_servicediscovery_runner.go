// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package service_discovery

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
	// Log stream based on job name in extra_apps.tpl:https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/test/ecs/ecs_sd/resources/extra_apps.tpl#L41
	LogStreamName = "prometheus-redis"

	// Scenario names
	ScenarioDockerLabel          = "dockerLabel"
	ScenarioTaskDefinitionList   = "taskDefinitionList"
	ScenarioServiceNameList      = "serviceNameList"
	ScenarioCombined             = "combined"
	ScenarioTargetDeduplication  = "targetDeduplication"
	ScenarioTargetDeduplication2 = "targetDeduplication2"
	ScenarioInvalidJobLabel      = "invalidJobLabel"

	// Custom values for specific scenarios
	CustomServiceNameJob = "prometheus-redis-service-name-list-job"
	CustomDockerLabelJob = "custom-docker-label-job-name"
	CustomClusterName    = "CustomClusterName"
)

//go:embed resources/emf_prometheus_redis_schema.json
var schema string

type ValidationConfig struct {
	LogStreamName string
	ClusterName   string
}

func (t ECSServiceDiscoveryTestRunner) getValidationConfig(env *environment.MetaData) ValidationConfig {
	switch t.scenarioName {
	case ScenarioCombined:
		return ValidationConfig{
			LogStreamName: LogStreamName,
			ClusterName:   CustomClusterName,
		}
	case ScenarioTargetDeduplication:
		return ValidationConfig{
			LogStreamName: CustomServiceNameJob,
			ClusterName:   env.EcsClusterName,
		}
	case ScenarioTargetDeduplication2:
		return ValidationConfig{
			LogStreamName: CustomDockerLabelJob,
			ClusterName:   env.EcsClusterName,
		}
	default:
		return ValidationConfig{
			LogStreamName: LogStreamName,
			ClusterName:   env.EcsClusterName,
		}
	}
}

type ECSServiceDiscoveryTestRunner struct {
	test_runner.BaseTestRunner
	scenarioName string
}

func (t ECSServiceDiscoveryTestRunner) GetTestName() string {
	if t.scenarioName != "" {
		return "ecs_servicediscovery_" + t.scenarioName
	}
	return "ecs_servicediscovery"
}

func (t ECSServiceDiscoveryTestRunner) GetAgentConfigFileName() string {
	switch t.scenarioName {
	case ScenarioDockerLabel:
		return ""
	case ScenarioTaskDefinitionList:
		return "./resources/config_task_definition_list.json"
	case ScenarioServiceNameList:
		return "./resources/config_service_name_list.json"
	case ScenarioCombined:
		return "./resources/config_combined.json"
	case ScenarioTargetDeduplication:
		return "./resources/config_target_deduplication.json"
	case ScenarioTargetDeduplication2:
		return "./resources/config_target_deduplication_2.json"
	case ScenarioInvalidJobLabel:
		return "./resources/config_invalid_joblabel.json"
	default:
		return ""
	}
}

func (t ECSServiceDiscoveryTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}

func (t ECSServiceDiscoveryTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, t.ValidateCloudWatchLogs())

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t ECSServiceDiscoveryTestRunner) ValidateCloudWatchLogs() status.TestResult {
	env := environment.GetEnvironmentMetaData()
	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, env.EcsClusterName)
	config := t.getValidationConfig(env)

	testResult := status.TestResult{
		Name:   fmt.Sprintf("Retrieve Test LogGroup: %s (scenario: %s)", logGroupName, t.scenarioName),
		Status: status.FAILED,
	}

	logGroupFound, err := t.ValidateLogGroupFormat(logGroupName, config)

	if logGroupFound {
		if err != nil {
			log.Printf("ECS ServiceDiscovery Test LogGroups invalid for scenario %s: %s\n", t.scenarioName, err)
			testResult.Name = fmt.Sprintf("Scenario %s: %s", t.scenarioName, err.Error())
			testResult.Status = status.FAILED
		} else {
			testResult.Status = status.SUCCESSFUL
		}
		awsservice.DeleteLogGroupAndStream(logGroupName, config.LogStreamName)
	}
	return testResult
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogGroupFormat(logGroupName string, config ValidationConfig) (bool, error) {
	start := time.Now()

	log.Printf("Scenario %s: Sleeping to allow metric collection in CloudWatch Logs.", t.scenarioName)
	time.Sleep(2 * time.Minute)

	log.Printf("Scenario %s: Searching for LogGroup: %s\n", t.scenarioName, logGroupName)

	for retries := 0; retries < MaxRetryCount; retries++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			end := time.Now()
			return true, t.ValidateLogsContent(logGroupName, config, start, end)
		}

		log.Printf("Scenario %s: Retry %d/%d: Log group not found. Waiting 20 seconds...\n", t.scenarioName, retries+1, MaxRetryCount)
		time.Sleep(20 * time.Second)
	}

	log.Printf("Scenario %s: ECS ServiceDiscovery Test has exhausted %v retry times", t.scenarioName, MaxRetryCount)
	return false, fmt.Errorf("scenario %s: test retries exhausted: %d", t.scenarioName, MaxRetryCount)
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogsContent(logGroupName string, config ValidationConfig, start time.Time, end time.Time) error {
	// Combined validation function for all fields
	combinedValidation := func(event types.OutputLogEvent) error {
		message := *event.Message

		// Job Name validation
		expectedJob := fmt.Sprintf("\"job\":\"%s\"", config.LogStreamName)
		if !strings.Contains(message, expectedJob) {
			return fmt.Errorf("scenario %s: expected job field %s not found in log: %s", t.scenarioName, expectedJob, message)
		}

		// ClusterName validation
		expectedCluster := fmt.Sprintf("\"ClusterName\":\"%s\"", config.ClusterName)
		if !strings.Contains(message, expectedCluster) {
			return fmt.Errorf("scenario %s: expected ClusterName field %s not found in log: %s", t.scenarioName, expectedCluster, message)
		}

		// Invalid/empty label removal validation
		if strings.Contains(message, "\"empty\":") {
			return fmt.Errorf("scenario %s: unexpected empty field found in metric: %s", t.scenarioName, message)
		}

		return nil
	}

	return awsservice.ValidateLogs(
		logGroupName,
		config.LogStreamName,
		&start,
		&end,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogSchema(awsservice.WithSchema(schema)),
			func(event types.OutputLogEvent) error {
				if strings.Contains(*event.Message, "CloudWatchMetrics") &&
					!strings.Contains(*event.Message, "\"Namespace\":\"ECS/ContainerInsights/Prometheus\"") {
					return fmt.Errorf("scenario %s: emf log found for non ECS/ContainerInsights/Prometheus namespace: %s", t.scenarioName, *event.Message)
				}
				return nil
			},
			combinedValidation,
		),
	)
}
