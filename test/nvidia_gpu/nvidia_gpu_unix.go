// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package nvidia_gpu

import (
	"errors"
	"fmt"
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

var (
	expectedNvidiaGPULinuxMetrics = []string{"mem_used_percent", "nvidia_smi_utilization_gpu", "nvidia_smi_utilization_memory", "nvidia_smi_power_draw", "nvidia_smi_temperature_gpu"}
)

func Validate() error {
	common.CopyFile(configLinuxJSON, configLinuxOutputPath)
	common.StartAgent(configLinuxOutputPath, true, false)

	time.Sleep(agentLinuxRuntime)
	common.StopAgent()

	dimensionFilter := awsservice.BuildDimensionFilterList(numberofLinuxAppendDimensions)
	for _, metricName := range expectedNvidiaGPULinuxMetrics {
		awsservice.ValidateMetric(metricName, metricLinuxNamespace, dimensionFilter)
	}

	if err := filesystem.CheckFileRights(agentLinuxLogPath); err != nil {
		return errors.New(fmt.Sprintf("CloudWatchAgent does not have privellege to write and read CWA's log: %v", err))
	}

	if err := filesystem.CheckFileOwnerRights(agentLinuxLogPath, agentLinuxPermission); err != nil {
		return errors.New(fmt.Sprintf("CloudWatchAgent does not have right to CWA's log: %v", err))
	}

	return nil
}
