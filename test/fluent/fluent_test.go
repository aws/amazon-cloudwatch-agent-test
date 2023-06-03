// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package fluent

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	namespace      = "Fluent"
	logStreamRetry = 20
	logEventCount  = 20
)

var (
	logGroups                      = []string{"dataplane", "host", "application"}
	hostLogFields                  = []string{"host", "ident", "message"}
	dataplaneLogFields             = []string{"message", "hostname", "systemd_unit"}
	applicationKubernetesLogFields = []string{"container_name", "namespace_name", "pod_name", "container_image", "pod_id", "host"}
	applicationLogFields           = []string{"log", "stream"}
)

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
		logGroupName = fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, logGroup)
		logGroupExist = awsservice.IsLogGroupExists(logGroupName)
		if !logGroupExist {
			t.Fatalf("fluent log group doesn't exsit: %s", logGroupName)
		}

		logStreams := getLogStreams(logGroupName)
		if len(logStreams) < 1 {
			t.Fatalf("fluent log streams are empty for log group: %s", logGroupName)
		}

	}

	t.Log("finishing EKS fluent log validation...")
}

func getLogStreams(logGroupName string) []types.LogStream {
	logStreams := make([]types.LogStream, 0)
	for i := 0; i < logStreamRetry; i++ {
		logStreams = awsservice.GetLogStreams(logGroupName)

		if len(logStreams) > 0 {
			break
		}
		time.Sleep(10 * time.Second)
	}

	return logStreams
}

func isLogValid() bool {
	return true
}
