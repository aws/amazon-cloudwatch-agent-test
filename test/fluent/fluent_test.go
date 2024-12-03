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

	currRetries := 0
	now := time.Now()
	for group, fieldsArr := range logGroupToKey {
		group = fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, group)
		for currRetries < logStreamRetry {
			if awsservice.IsLogGroupExists(group) {
				break
			} else {
				currRetries++
				time.Sleep(time.Duration(currRetries) * time.Second)
			}
		}
		if currRetries >= logStreamRetry {
			t.Fatalf("fluent log group doesn't exsit: %s", group)
		}

		streams := awsservice.GetLogStreams(group)
		if len(streams) == 0 {
			t.Fatalf("fluent log streams are empty for log group: %s", group)
		}

		err := awsservice.ValidateLogs(
			group,
			*(streams[0].LogStreamName),
			nil,
			&now,
			awsservice.AssertLogsNotEmpty(),
			func(events []types.OutputLogEvent) error {
				// only 1 log message gets validated
				// log message must include expected fields, and there could be more than 1 set of expected fields per log group
				var found bool
				for _, fields := range fieldsArr {
					var match int
					for _, field := range fields {
						if strings.Contains(*events[0].Message, "\""+field+"\"") {
							match += 1
						}
					}
					if match == len(fields) {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("fluent log entry doesn't include expected message fields: %s", *events[0].Message)
				}
				return nil
			},
		)

		if err != nil {
			t.Fatalf("failed validation for log group %s: %v", group, err)
		}
	}

	t.Log("finishing EKS fluent log validation...")
}
