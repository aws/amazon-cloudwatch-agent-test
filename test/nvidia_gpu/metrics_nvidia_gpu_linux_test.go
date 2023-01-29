// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metrics_nvidia_gpu

import (
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configLinuxJSON               = "resources/config_linux.json"
	metricLinuxNamespace          = "NvidiaGPULinuxTest"
	configLinuxOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentLinuxLogPath             = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	agentLinuxRuntime             = 2 * time.Minute
	agentLinuxPermission          = "root"
	numberofLinuxAppendDimensions = 1
)

var expectedNvidiaGPULinuxMetrics = []string{"mem_used_percent", "nvidia_smi_utilization_gpu", "nvidia_smi_utilization_memory", "nvidia_smi_power_draw", "nvidia_smi_temperature_gpu"}

func TestNvidiaGPU(t *testing.T) {
	t.Run("Basic configuration testing for both metrics and logs", func(t *testing.T) {
		common.CopyFile(configLinuxJSON, configLinuxOutputPath)
		common.StartAgent(configLinuxOutputPath, true)

		time.Sleep(agentLinuxRuntime)
		t.Logf("Agent has been running for : %s", agentLinuxRuntime.String())
		common.StopAgent()

		dimensionFilter, err := awsservice.BuildDimensionFilterList(numberofLinuxAppendDimensions)

		if err != nil {
			t.Fatalf("Failed to build dimension filter list: %v", err)
		}

		for _, metricName := range expectedNvidiaGPULinuxMetrics {
			if err := awsservice.AWS.CwmAPI.ValidateMetrics(metricName, metricLinuxNamespace, dimensionFilter); err != nil {
				t.Fatalf("Failed to get the corresponding metrics %s: %v", metricName, err)
			}
		}

		if err := filesystem.CheckFileRights(agentLinuxLogPath); err != nil {
			t.Fatalf("CloudWatchAgent does not have privellege to write and read CWA's log: %v", err)
		}

		if err := filesystem.CheckFileOwnerRights(agentLinuxLogPath, agentLinuxPermission); err != nil {
			t.Fatalf("CloudWatchAgent does not have right to CWA's log: %v", err)
		}

	})
}
