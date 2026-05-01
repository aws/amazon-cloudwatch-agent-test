// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package efa_ec2

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configLinuxJSON               = "resources/config_linux.json"
	metricLinuxNamespace          = "EfaEC2LinuxTest"
	configLinuxOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentLinuxLogPath             = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	agentLinuxRuntime             = 2 * time.Minute
	agentLinuxPermission          = "root"
	numberofLinuxAppendDimensions = 1
)

var (
	expectedEfaEC2LinuxMetrics = []string{"efa_tx_bytes", "efa_rx_bytes", "efa_tx_pkts", "efa_rx_pkts", "efa_rx_dropped", "efa_rdma_read_bytes", "efa_rdma_write_bytes", "efa_send_bytes", "efa_recv_bytes"}
)

func Validate() error {
	common.CopyFile(configLinuxJSON, configLinuxOutputPath)
	common.StartAgent(configLinuxOutputPath, true, false)

	time.Sleep(agentLinuxRuntime)
	common.StopAgent()

	dimensionFilter := awsservice.BuildDimensionFilterList(numberofLinuxAppendDimensions)
	for _, metricName := range expectedEfaEC2LinuxMetrics {
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
