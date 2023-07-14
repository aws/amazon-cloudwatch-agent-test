// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

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
	configOutputPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	namespace        = "MultiConfigWindowsTest"
	agentRuntime     = 2 * time.Minute
)

func Validate() error {
	agentConfigurations := []string{"resources/windows/WindowsCompleteConfig.json", "resources/windows/WindowsMemoryOnlyConfig.json"}

	AppendConfigs(agentConfigurations, configOutputPath)

	time.Sleep(agentRuntime)
	log.Printf("Agent has been running for : %s", agentRuntime.String())
	err := common.StopAgent()
	if err != nil {
		log.Printf("Stopping agent failed: %v", err)
		return err
	}

	// test for cloud watch metrics
	ec2InstanceId := awsservice.GetInstanceId()
	expectedDimensions := []types.DimensionFilter{
		types.DimensionFilter{
			Name:  aws.String("InstanceId"),
			Value: aws.String(ec2InstanceId),
		},
	}

	expectedMetrics := []string{"% Committed Bytes In Use", "% InterruptTime", "% Disk Time"}
	for _, expectedMetric := range expectedMetrics {
		err = awsservice.ValidateMetric(expectedMetric, namespace, expectedDimensions)
	}
	if err != nil {
		log.Printf("CloudWatch Agent append config not working : %v", err)
		return err
	}
	return nil
}
