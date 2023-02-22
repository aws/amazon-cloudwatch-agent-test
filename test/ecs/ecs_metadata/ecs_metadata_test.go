// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_metadata

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/qri-io/jsonschema"
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
	rs := jsonschema.Must(schema)

	start := time.Now()

	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, *clusterName)

	var logGroupFound bool
	for currentRetry := 1; ; currentRetry++ {

		if currentRetry == RetryTime {
			t.Fatalf("Test metadata has exhausted %v retry time", RetryTime)
		}

		exists := awsservice.IsLogGroupExists(logGroupName)
		if !exists {
			log.Printf("Current retry: %v/%v and begin to sleep for 20s \n", currentRetry, RetryTime)
			time.Sleep(20 * time.Second)
			continue
		}

		end := time.Now()

		ok, err := awsservice.ValidateLogs(logGroupName, LogStreamName, &start, &end, func(logs []string) bool {
			if len(logs) < 1 {
				return false
			}
			for _, l := range logs {
				keyErrors, e := rs.ValidateBytes(context.Background(), []byte(l))
				if e != nil {
					log.Println("failed to execute schema validator:", e)
					return false
				} else if len(keyErrors) > 0 {
					log.Printf("failed schema validation: %v\n", keyErrors)
					return false
				}
			}
			return true
		})
		assert.NoError(t, err)
		assert.True(t, ok)

		break
	}

	if logGroupFound {
		awsservice.DeleteLogGroupAndStream(logGroupName, LogStreamName)
	}
}
