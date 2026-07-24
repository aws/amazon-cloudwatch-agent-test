// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package service_discovery

import (
	_ "embed"
	"errors"
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
	// logEventPropagationTimeout bounds phase 2 (waiting for events once the log group
	// exists) by wall clock rather than a retry count, since each attempt can block ~90s
	// internally in GetLogsSince on a not-yet-created stream.
	logEventPropagationTimeout = 5 * time.Minute
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
	// Log stream based on job name in extra_apps.tpl:https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/test/ecs/service_discovery/resources/extra_apps.tpl#L41
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

// ValidateLogGroupFormat waits for the scenario's log group, then validates its events. The
// returned bool reports whether the log group was located -- i.e. whether the caller should
// run cleanup -- NOT whether validation passed; inspect the error for pass/fail. It returns
// (false, err) only when the group never appeared.
func (t ECSServiceDiscoveryTestRunner) ValidateLogGroupFormat(logGroupName string, config ValidationConfig) (bool, error) {
	start := time.Now()

	log.Printf("Scenario %s: Sleeping to allow metric collection in CloudWatch Logs.", t.scenarioName)
	time.Sleep(1 * time.Minute)

	// Phase 1: wait for the log group to be created.
	log.Printf("Scenario %s: Searching for LogGroup: %s\n", t.scenarioName, logGroupName)
	logGroupFound := false
	for retries := 0; retries < MaxRetryCount; retries++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			logGroupFound = true
			break
		}
		log.Printf("Scenario %s: Retry %d/%d: log group not found. Waiting 20 seconds...\n", t.scenarioName, retries+1, MaxRetryCount)
		time.Sleep(20 * time.Second)
	}
	if !logGroupFound {
		return false, fmt.Errorf("scenario %s: log group %s not found after %d retries", t.scenarioName, logGroupName, MaxRetryCount)
	}

	// Phase 2: group exists; validate content, retrying while the target stream is still
	// catching up -- empty (ErrNoLogEvents) or not yet created (ResourceNotFoundException).
	// Any other error (schema/job/cluster mismatch, or a transient AWS error) fails fast by
	// design. Bounded by a wall-clock deadline, not a retry count, because each attempt can
	// itself block ~90s inside GetLogsSince (StandardRetries x 30s) on a missing stream.
	deadline := time.Now().Add(logEventPropagationTimeout)
	var lastErr error
	for {
		end := time.Now()
		err := t.ValidateLogsContent(logGroupName, config, start, end)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, awsservice.ErrNoLogEvents) && !awsservice.IsResourceNotFoundException(err) {
			// true = log group located (caller should clean up), not "validation passed".
			return true, err
		}
		lastErr = err
		if time.Now().After(deadline) {
			break
		}
		log.Printf("Scenario %s: log events not available yet after %s; waiting 20 seconds...\n", t.scenarioName, time.Since(start).Round(time.Second))
		time.Sleep(20 * time.Second)
	}

	// Group exists but never produced events within the deadline: return true so the caller
	// still cleans it up and reports the real cause rather than a misleading "not found".
	return true, fmt.Errorf("scenario %s: no log events within %s: %w", t.scenarioName, logEventPropagationTimeout, lastErr)
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
