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
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configWindowsJSON               = "resources/config_windows.json"
	metricWindowsnamespace          = "NvidiaGPUWindowsTest"
	configWindowsOutputPath         = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath             = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentWindowsRuntime             = 3 * time.Minute
	numberofWindowsAppendDimensions = 1
)

var (
	envMetaDataStrings              = &(environment.MetaDataStrings{})
	expectedNvidiaGPUWindowsMetrics = []string{"Memory % Committed Bytes In Use", "nvidia_smi utilization_gpu", "nvidia_smi utilization_memory", "nvidia_smi power_draw", "nvidia_smi temperature_gpu"}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
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

	var err error
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
}
