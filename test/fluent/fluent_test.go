// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package fluent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const logStreamRetry = 10

// fluent log group with expected log message fields
var logGroupToKey = map[string][][]string{
	"dataplane": {
		{"dataplane", "host", "application"},
		{"message", "hostname", "systemd_unit"},
		{"log", "stream"},
	},
	"host": {
		{"host", "ident", "message"},
	},
	"application": {
		{"container_name", "namespace_name", "pod_name", "container_image", "pod_id", "host"},
		{"log", "stream"},
	},
}

// fluent log group with expected log message fields on Windows node.
var logGroupToKeyWindows = map[string][][]string{
	"dataplane": {
		{"log", "file_name"},
		{"SourceName", "Message", "ComputerName"},
	},
	"host": {
		{"SourceName", "Message", "ComputerName"},
	},
	"application": {
		{"log"},
	},
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestFluentLogs(t *testing.T) {
	t.Log("starting EKS fluent log validation...")
	env := environment.GetEnvironmentMetaData()

	if env.InstancePlatform == "windows" {
		logGroupToKey = logGroupToKeyWindows
	}

	timeout := time.Now().Add(5 * time.Minute)

	for group, fieldsArr := range logGroupToKey {
		group = fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, group)

		currRetries := 0
		for currRetries < logStreamRetry {
			if awsservice.IsLogGroupExists(group) {
				break
			}
			currRetries++
			time.Sleep(time.Duration(currRetries) * time.Second)
		}
		if currRetries >= logStreamRetry {
			t.Fatalf("fluent log group doesn't exist: %s", group)
		}

		var lastErr error
		for time.Now().Before(timeout) {
			streams := awsservice.GetLogStreams(group)
			if len(streams) == 0 {
				time.Sleep(10 * time.Second)
				continue
			}

			err := awsservice.ValidateLogs(
				group,
				*(streams[0].LogStreamName),
				nil,
				&timeout,
				awsservice.AssertLogsNotEmpty(),
				func(events []types.OutputLogEvent) error {
					if len(events) == 0 {
						return fmt.Errorf("no log events found")
					}

					for _, event := range events {
						for _, fields := range fieldsArr {
							var match int
							for _, field := range fields {
								if strings.Contains(*event.Message, "\""+field+"\"") {
									match += 1
								}
							}
							if match == len(fields) {
								return nil
							}
						}
					}
					return fmt.Errorf("no matching log entry found with expected fields")
				},
			)

			if err == nil {
				break
			}

			lastErr = err
			time.Sleep(10 * time.Second)
		}

		if lastErr != nil {
			t.Fatalf("failed validation for log group %s: %v", group, lastErr)
		}
	}

	t.Log("finishing EKS fluent log validation...")
}
