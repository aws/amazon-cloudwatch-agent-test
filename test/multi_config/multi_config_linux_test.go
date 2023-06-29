// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package multi_config

import (
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	namespace        = "MultiConfigTest"
)

// Let the agent run for 2 minutes. This will give agent enough time to call server
const agentRuntime = 2 * time.Minute

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
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
