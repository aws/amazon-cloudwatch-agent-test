// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package multi_config

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	namespace        = "MultiConfigTest"
)

// Let the agent run for 2 minutes. This will give agent enough time to call server
const agentRuntime = 2 * time.Minute

func Validate() error {

	agentConfigurations := []string{"resources/linux/LinuxCpuOnlyConfig.json", "resources/linux/LinuxMemoryOnlyConfig.json", "resources/linux/LinuxDiskOnlyConfig.json"}

	AppendConfigs(agentConfigurations, configOutputPath)

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

	expectedMetrics := []string{"memory", "cpu_time_active_userdata", "disk_free"}
	for _, expectedMetric := range expectedMetrics {
		err := awsservice.ValidateMetric(expectedMetric, namespace, expectedDimensions)
		if err != nil {
			log.Printf("CloudWatch Agent append config not working : %v", err)
			return err
		}
	}
	return nil
}
