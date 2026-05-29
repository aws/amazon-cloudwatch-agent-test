// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package efa_ec2

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

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
	expectedEfaEC2LinuxMetrics    = []string{"efa_tx_bytes", "efa_rx_bytes", "efa_tx_pkts", "efa_rx_pkts", "efa_rx_dropped", "efa_rdma_read_bytes", "efa_rdma_write_bytes", "efa_send_bytes", "efa_recv_bytes"}
	expectedEfaDimensionNames     = []string{"device", "port", "eniId"}
)

func Validate() error {
	common.CopyFile(configLinuxJSON, configLinuxOutputPath)
	common.StartAgent(configLinuxOutputPath, true, false)

	time.Sleep(agentLinuxRuntime)
	common.StopAgent()

	dimensionFilter := awsservice.BuildDimensionFilterList(numberofLinuxAppendDimensions)
	for _, metricName := range expectedEfaEC2LinuxMetrics {
		if err := awsservice.ValidateMetric(metricName, metricLinuxNamespace, dimensionFilter); err != nil {
			return err
		}
	}

	// Validate that EFA metrics use short dimension names (device, port, eniId)
	// instead of OTel-style dotted names (aws.efa.device, aws.efa.port, aws.efa.eni.id)
	if err := validateEfaDimensionNames(); err != nil {
		return err
	}

	if err := filesystem.CheckFileRights(agentLinuxLogPath); err != nil {
		return fmt.Errorf("CloudWatchAgent does not have privilege to write and read CWA's log: %v", err)
	}

	if err := filesystem.CheckFileOwnerRights(agentLinuxLogPath, agentLinuxPermission); err != nil {
		return fmt.Errorf("CloudWatchAgent does not have right to CWA's log: %v", err)
	}

	return nil
}

// validateEfaDimensionNames verifies that EFA metrics are published with short
// dimension names (device, port, eniId) rather than OTel-style dotted names.
func validateEfaDimensionNames() error {
	dimFilter := make([]types.DimensionFilter, len(expectedEfaDimensionNames))
	for i, name := range expectedEfaDimensionNames {
		dimFilter[i] = types.DimensionFilter{Name: aws.String(name)}
	}

	// Check at least one EFA metric exists with the expected short dimension names
	if err := awsservice.ValidateMetric(expectedEfaEC2LinuxMetrics[0], metricLinuxNamespace, dimFilter); err != nil {
		return fmt.Errorf("EFA metrics missing expected short dimension names %v: %w", expectedEfaDimensionNames, err)
	}
	return nil
}
