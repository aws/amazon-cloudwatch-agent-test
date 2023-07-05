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

func Validate(t *testing.T) error {
	agentConfigurations := []string{"resources/WindowsLogOnlyConfig.json", "resources/WindowsMemoryOnlyConfig.json"}

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

	expectedMetrics := []string{"% Committed Bytes In Use", "% InterruptTime"}
	for _, expectedMetric := range expectedMetrics {
		err = awsservice.ValidateMetric(expectedMetric, namespace, expectedDimensions)
	}
	if err != nil {
		log.Printf("CloudWatch Agent apped config not working : %v", err)
		return err
	}
	return nil
}
