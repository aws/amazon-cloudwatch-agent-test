// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package fluent

import (
	"fmt"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	namespace      = "Fluent"
	logGroupPrefix = "/aws/containerinsights"
)

var logGroups = []string{"dataplane", "host", "application"}
var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestFluentLogs(t *testing.T) {
	t.Log("starting EKS fluent log validation...")
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)

	var logGroupExist bool
	var logGroupName string
	for _, logGroup := range logGroups {
		logGroupName = fmt.Sprintf("%s/%s/%s", logGroupPrefix, env.EKSClusterName, logGroup)
		logGroupExist = awsservice.IsLogGroupExists(logGroupName)
		if !logGroupExist {
			t.Fatalf("fluent log group doesn't exsit: %s", logGroupName)
		}

		logStreams := awsservice.GetLogStreams(logGroupName)
		if len(logStreams) < 1 {
			t.Fatalf("fluent log streams are empty for log group: %s", logGroupName)
		}
	}

	t.Log("finishing EKS fluent log validation...")
}
