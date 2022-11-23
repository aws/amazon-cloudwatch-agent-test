// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows && integration
// +build windows,integration

package metrics_nvidia_gpu

import (
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/agent"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/aws"
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
		err := agent.CopyFile(configWindowsJSON, configWindowsOutputPath)

		if err != nil {
			t.Fatalf(err.Error())
		}

		err = agent.StartAgent(configWindowsOutputPath, true)

		if err != nil {
			t.Fatalf(err.Error())
		}

		time.Sleep(agentWindowsRuntime)
		t.Logf("Agent has been running for : %s", agentWindowsRuntime.String())
		err = agent.StopAgent()

		if err != nil {
			t.Fatalf(err.Error())
		}

		dimensionFilter := aws.BuildDimensionFilterList(numberofWindowsAppendDimensions)
		for _, metricName := range expectedNvidiaGPUWindowsMetrics {
			aws.ValidateMetrics(t, metricName, metricWindowsnamespace, dimensionFilter)
		}

		err = filesystem.CheckFileRights(agentWindowsLogPath)
		if err != nil {
			t.Fatalf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
		}

	})
}
