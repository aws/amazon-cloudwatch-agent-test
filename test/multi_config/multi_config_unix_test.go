// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package multi_config

import (
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	namespace        = "MultiConfigTest"
)

// Let the agent run for 2 minutes. This will give agent enough time to call server
const agentRuntime = 2 * time.Minute

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestMultipleConfig(t *testing.T) {

	agentConfigurations := []string{"resources/LinuxLogOnlyConfig.json", "resources/LinuxMemoryOnlyConfig.json"}

	for index, agentConfig := range agentConfigurations {
		common.CopyFile(agentConfig, configOutputPath)
		log.Printf(configOutputPath)
		if index == 0 {
			common.StartAgent(configOutputPath, true, false)
		} else {
			common.StartAgentWithMultiConfig(configOutputPath, true, false)
		}
		time.Sleep(30 * time.Second)
	}

	time.Sleep(agentRuntime)
	log.Printf("Agent has been running for : %s", agentRuntime.String())
	common.StopAgent()

	// test for cloud watch metrics
	ec2InstanceId := awsservice.GetInstanceId()
	expectedDimensions := []types.DimensionFilter{
		types.DimensionFilter{
			Name:  aws.String("InstanceId"),
			Value: aws.String(ec2InstanceId),
		},
	}

	expectedMetrics := []string{"mem_used_percent"}
	for _, expectedMetric := range expectedMetrics {
		awsservice.ValidateMetric(expectedMetric, namespace, expectedDimensions)
	}

	ok, err := awsservice.ValidateLogs(logGroup, logStream, &start, &end, func(logs []string) bool {
		if len(logs) != len(lines) {
			return false
		}

		for i := 0; i < len(logs); i++ {
			expected := strings.ReplaceAll(lines[i], "'", "\"")
			actual := strings.ReplaceAll(logs[i], "'", "\"")
			if expected != actual {
				return false
			}
		}

		return true
	}
}
