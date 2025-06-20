// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_sd

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"log"
	"strings"
	"time"
)

const (
	MaxRetryCount = 15
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
	// Log stream based on job name in extra_apps.tpl:https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/test/ecs/ecs_sd/resources/extra_apps.tpl#L41
	LogStreamName = "prometheus-redis"
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
	testResults = append(testResults, t.ValidateCloudWatchLogs())

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t ECSServiceDiscoveryTestRunner) ValidateCloudWatchLogs() status.TestResult {
	env := environment.GetEnvironmentMetaData()
	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, env.EcsClusterName)

	testResult := status.TestResult{
		Name:   fmt.Sprintf("Retrieve Test LogGroup: %s", logGroupName),
		Status: status.FAILED,
	}

	logGroupFound, err := t.ValidateLogGroupFormat(logGroupName)

	if logGroupFound {
		if err != nil {
			log.Printf("ECS ServiceDiscovery Test LogGroups invalid\n")
			testResult.Name = err.Error()
			testResult.Status = status.FAILED
		} else {
			testResult.Status = status.SUCCESSFUL
		}
		awsservice.DeleteLogGroupAndStream(logGroupName, LogStreamName)
	}
	return testResult
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogGroupFormat(logGroupName string) (bool, error) {
	start := time.Now()

	for retries := 0; retries < MaxRetryCount; retries++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			end := time.Now()
			return true, t.ValidateLogsContent(logGroupName, start, end)
		}

		log.Printf("Retry %d/%d: Log group not found. Waiting 20 seconds...\n", retries+1, MaxRetryCount)
		time.Sleep(20 * time.Second)
	}

	log.Printf("ECS ServiceDiscovery Test has exhausted %v retry times", MaxRetryCount)
	return false, fmt.Errorf("Test Retries Exhausted: %d", MaxRetryCount)
}

func (t ECSServiceDiscoveryTestRunner) ValidateLogsContent(logGroupName string, start time.Time, end time.Time) error {
	return awsservice.ValidateLogs(
		logGroupName,
		LogStreamName,
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
			awsservice.AssertLogContainsSubstring("\"job\":\"prometheus-redis\""),
		),
	)
}
