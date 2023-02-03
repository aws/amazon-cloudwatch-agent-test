// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package metrics_nvidia_gpu

import (
	"testing"
	"time"

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

var expectedNvidiaGPUWindowsMetrics = []string{"Memory % Committed Bytes In Use", "nvidia_smi utilization_gpu", "nvidia_smi utilization_memory", "nvidia_smi power_draw", "nvidia_smi temperature_gpu"}

func TestNvidiaGPUWindows(t *testing.T) {
	t.Run("Run CloudWatchAgent with Nvidia-smi on Windows", func(t *testing.T) {
		err := common.CopyFile(configWindowsJSON, configWindowsOutputPath)

		if err != nil {
			t.Fatalf(err.Error())
		}

		err = common.StartAgent(configWindowsOutputPath, true)

		if err != nil {
			t.Fatalf(err.Error())
		}

		time.Sleep(agentWindowsRuntime)
		t.Logf("Agent has been running for : %s", agentWindowsRuntime.String())
		err = common.StopAgent()

		if err != nil {
			t.Fatalf(err.Error())
		}

		dimensionFilter, err := awsservice.BuildDimensionFilterList(numberofWindowsAppendDimensions)

		if err != nil {
			t.Fatalf("Failed to build dimension filter list: %v", err)
		}

		for _, metricName := range expectedNvidiaGPUWindowsMetrics {
			if err := awsservice.ValidateMetrics(metricName, metricWindowsnamespace, dimensionFilter); err != nil {
				t.Fatalf("Failed to get the corresponding metrics %s: %v", metricName, err)
			}
		}

		err = filesystem.CheckFileRights(agentWindowsLogPath)
		if err != nil {
			t.Fatalf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
		}

	})
}
