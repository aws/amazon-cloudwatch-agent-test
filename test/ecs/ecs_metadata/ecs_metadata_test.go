// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_metadata

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

// Purpose: Detect the changes in metadata endpoint for ECS Container Agent https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/util/ecsutil/ecsutil.go#L67-L75
// Implementation: Checking if a log group's the format(https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/translate/logs/metrics_collected/prometheus/ruleLogGroupName.go#L33)
// exists or not  since the log group's format has the scrapping cluster name from metadata endpoint.

const (
	RetryTime = 15
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/translate/logs/metrics_collected/prometheus/ruleLogGroupName.go#L33
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
	// Log stream based on job name: https://github.com/khanhntd/amazon-cloudwatch-agent/blob/ecs_metadata/integration/test/ecs/ecs_metadata/resources/extra_apps.tpl#L41
	LogStreamName = "prometheus-redis"
)

var clusterName = flag.String("clusterName", "", "Please provide the os preference, valid value: windows/linux.")

//go:embed resources/emf_prometheus_redis_schema.json
var schema string

func TestValidatingCloudWatchLogs(t *testing.T) {
	start := time.Now()

	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, *clusterName)

	var logGroupFound bool
	for currentRetry := 1; ; currentRetry++ {

		if currentRetry == RetryTime {
			t.Fatalf("Test metadata has exhausted %v retry time", RetryTime)
		}

		if !awsservice.IsLogGroupExists(logGroupName) {
			log.Printf("Current retry: %v/%v and begin to sleep for 20s \n", currentRetry, RetryTime)
			time.Sleep(20 * time.Second)
			continue
		}

		end := time.Now()

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

		break
	}

	if logGroupFound {
		awsservice.DeleteLogGroupAndStream(logGroupName, LogStreamName)
	}
}
