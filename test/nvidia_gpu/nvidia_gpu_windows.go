// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package nvidia_gpu

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// agent config json file in Temp dir gets written by terraform
	configWindowsJSON               = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\agent_config.json"
	metricWindowsnamespace          = "NvidiaGPUWindowsTest"
	configWindowsOutputPath         = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath             = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentWindowsRuntime             = 3 * time.Minute
	numberofWindowsAppendDimensions = 1
)

var (
	expectedNvidiaGPUWindowsMetrics = []string{"Memory % Committed Bytes In Use", "nvidia_smi utilization_gpu", "nvidia_smi utilization_memory", "nvidia_smi power_draw", "nvidia_smi temperature_gpu"}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func Validate() error {
	err := common.CopyFile(configWindowsJSON, configWindowsOutputPath)
	if err != nil {
		log.Printf("Copying agent config file failed: %v", err)
		return err
	}

	err = common.StartAgent(configWindowsOutputPath, true, false)
	if err != nil {
		log.Printf("Starting agent failed: %v", err)
		return err
	}

	time.Sleep(agentWindowsRuntime)
	log.Printf("Agent has been running for : %s", agentWindowsRuntime.String())

	err = common.StopAgent()
	if err != nil {
		log.Printf("Stopping agent failed: %v", err)
		return err
	}

	dimensionFilter := awsservice.BuildDimensionFilterList(numberofWindowsAppendDimensions)
	for _, metricName := range expectedNvidiaGPUWindowsMetrics {
		err = awsservice.ValidateMetric(metricName, metricWindowsnamespace, dimensionFilter)
		if err != nil {
			log.Printf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
			return err
		}
	}

	err = filesystem.CheckFileRights(agentWindowsLogPath)
	if err != nil {
		log.Printf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
		return err
	}

	return nil
}
