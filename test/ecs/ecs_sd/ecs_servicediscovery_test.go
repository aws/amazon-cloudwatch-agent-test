// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_sd

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

/*
Purpose:
1) Validate ECS ServiceDiscovery via DockerLabels by publishing Prometheus EMF to CW  https://github.com/aws/amazon-cloudwatch-agent/blob/main/internal/ecsservicediscovery/README.md
2) Detect the changes in metadata endpoint for ECS Container Agent https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/util/ecsutil/ecsutil.go#L67-L75


Implementation:
1) Check if the LogGroupFormat correctly scrapes the clusterName from metadata endpoint (https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38)
2) Check if expected Prometheus EMF data is correctly published as logs and metrics to CloudWatch
*/

const (
	RetryTime = 15
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
	// Log stream based on job name in extra_apps.tpl:https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/test/ecs/ecs_sd/resources/extra_apps.tpl#L41
	LogStreamName = "prometheus-redis"
)

var clusterName = flag.String("clusterName", "", "Please provide the os preference, valid value: windows/linux.")

//go:embed resources/emf_prometheus_redis_schema.json
var schema string

func TestValidatingCloudWatchLogs(t *testing.T) {

	logGroupName, logGroupFound, start, end := ValidateLogGroupFormat(t)

	ValidateLogsContent(t, logGroupName, start, end)

	if logGroupFound {
		awsservice.DeleteLogGroupAndStream(logGroupName, LogStreamName)
	}
}

func ValidateLogGroupFormat(t *testing.T) (string, bool, time.Time, time.Time) {
	start := time.Now()
	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, *clusterName)

	var logGroupFound bool
	for currentRetry := 1; ; currentRetry++ {

		if currentRetry == RetryTime {
			t.Fatalf("Test has exhausted %v retry time", RetryTime)
		}

		if !awsservice.IsLogGroupExists(logGroupName) {
			log.Printf("Current retry: %v/%v and begin to sleep for 20s \n", currentRetry, RetryTime)
			time.Sleep(20 * time.Second)
			continue
		}
		break
	}
	end := time.Now()
	return logGroupName, logGroupFound, start, end
}

func ValidateLogsContent(t *testing.T, logGroupName string, start time.Time, end time.Time) {
	err := awsservice.ValidateLogs(
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
	assert.NoError(t, err)
}
