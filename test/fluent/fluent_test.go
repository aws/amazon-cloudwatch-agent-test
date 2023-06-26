// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package fluent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const logStreamRetry = 20

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

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestFluentLogs(t *testing.T) {
	t.Log("starting EKS fluent log validation...")
	env := environment.GetEnvironmentMetaData()

	now := time.Now()
	for group, fieldsArr := range logGroupToKey {
		group := fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, group)
		if !awsservice.IsLogGroupExists(group) {
			t.Fatalf("fluent log group doesn't exsit: %s", group)
		}

		streams := awsservice.GetLogStreams(group)
		if len(streams) < 1 {
			t.Fatalf("fluent log streams are empty for log group: %s", group)
		}

		ok, err := awsservice.ValidateLogs(group, *(streams[0].LogStreamName), nil, &now, func(logs []string) bool {
			if len(logs) < 1 {
				return false
			}

			// only 1 log message gets validate
			// log message must include expected fields, and there could be more than 1 set of expected fields  per log group
			var found = false
			for _, fields := range fieldsArr {
				match := 0
				for _, field := range fields {
					if strings.Contains(logs[0], "\""+field+"\"") {
						match += 1
					}
				}
				if match == len(fields) {
					found = true
					break
				}
			}
			return found
		})

		if err != nil || !ok {
			t.Fatalf("fluent log entry doesn't include expected message fields for logGroup: %s", group)
		}
	}

	t.Log("finishing EKS fluent log validation...")
}
