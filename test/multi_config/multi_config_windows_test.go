// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package multi_config

import (
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	configOutputPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	namespace        = "MultiConfigWindowsTest"
	agentRuntime     = 2 * time.Minute
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestMultipleConfigWindows(t *testing.T) {
	agentConfigurations := []string{"resources/WindowsLogOnlyConfig.json", "resources/WindowsMemoryOnlyConfig.json"}
	for index, agentConfig := range agentConfigurations {
		common.CopyFile(agentConfig, configOutputPath)
		log.Printf(configOutputPath)
		if index == 0 {
			common.StartAgent(configOutputPath, true, false)
		} else {
			common.StartAgentWithMultiConfig(configOutputPath, true, false)
		}
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
		awsservice.ValidateMetrics(t, expectedMetric, namespace, expectedDimensions)
	}
}
