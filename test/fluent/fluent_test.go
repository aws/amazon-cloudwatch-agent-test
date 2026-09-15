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
		{"log", "az", "ec2_instance_id"},
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

	for group, fieldsArr := range logGroupToKey {
		group = fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, group)

		// Unified poll loop: log-group creation, log-stream availability, and
		// content validation all share a single budget. This avoids a short
		// group-existence gate expiring before a slow producer has had a chance to create the group.
		maxRetries := 30
		validated := false
		var lastErr error
		for retry := 0; retry < maxRetries; retry++ {
			if !awsservice.IsLogGroupExists(group) {
				lastErr = fmt.Errorf("log group %s not created yet", group)
				t.Logf("Log group %s not created yet, waiting... (attempt %d/%d)", group, retry+1, maxRetries)
				time.Sleep(10 * time.Second)
				continue
			}

			streams := awsservice.GetLogStreams(group)
			if len(streams) == 0 {
				lastErr = fmt.Errorf("no log streams found for %s", group)
				t.Logf("No log streams found for %s, waiting... (attempt %d/%d)", group, retry+1, maxRetries)
				time.Sleep(10 * time.Second)
				continue
			}

			err := awsservice.ValidateLogs(
				group,
				*(streams[0].LogStreamName),
				nil,
				nil,
				awsservice.AssertLogsNotEmpty(),
				func(events []types.OutputLogEvent) error {
					// only 1 log message gets validated
					// log message must include expected fields, and there could be more than 1 set of expected fields per log

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
					return fmt.Errorf("no log entries found with expected message fields in %d events", len(events))
				},
			)

			if err == nil {
				validated = true
				break
			}

			lastErr = err
			t.Logf("Waiting for valid logs to appear in %s... (attempt %d/%d): %v", group, retry+1, maxRetries, err)
			time.Sleep(10 * time.Second)
		}

		if !validated {
			t.Fatalf("failed validation for log group %s within %d retries: %v", group, maxRetries, lastErr)
		}
	}

	t.Log("finishing EKS fluent log validation...")
}
