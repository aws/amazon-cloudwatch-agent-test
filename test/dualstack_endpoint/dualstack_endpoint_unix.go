// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dualstack_endpoint

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentConfigPath  = "agent_configs/dualstack_config.json"
	stsConfigPath    = "agent_configs/dualstack_sts_config.json"
	agentRuntime     = 1 * time.Minute
	stsAgentRuntime  = 1 * time.Minute
)

var expectedEndpoints = []string{
	"ec2.us-west-2.api.aws",
	"logs.us-west-2.api.aws",
	"monitoring.us-west-2.api.aws",
}

var expectedStsEndpoint = "sts.us-west-2.api.aws"

func ValidateDualstackEndpoints() error {
	err := testStsEndpoint()
	if err != nil {
		return fmt.Errorf("STS endpoint test failed: %w", err)
	}

	err = testServiceEndpoints()
	if err != nil {
		return fmt.Errorf("service endpoints test failed: %w", err)
	}

	return nil
}

func testStsEndpoint() error {
	common.CopyFile(stsConfigPath, configOutputPath)

	err := common.StartAgent(configOutputPath, false, false)
	if err != nil {
		return fmt.Errorf("failed to start agent for STS test: %w", err)
	}

	time.Sleep(stsAgentRuntime)
	common.StopAgent()

	return checkStsEndpoint()
}

func testServiceEndpoints() error {
	common.CopyFile(agentConfigPath, configOutputPath)

	err := common.StartAgent(configOutputPath, false, false)
	if err != nil {
		return fmt.Errorf("failed to start agent for service endpoints test: %w", err)
	}

	time.Sleep(agentRuntime)
	common.StopAgent()

	return checkServiceEndpoints()
}

func checkStsEndpoint() error {
	cmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log | grep api.aws"
	output, err := common.RunCommand(cmd)
	if err != nil {
		log.Printf("Failed to read agent logs: %v", err)
		return fmt.Errorf("failed to read agent logs: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("no dualstack endpoints found in agent logs")
	}

	if !strings.Contains(output, expectedStsEndpoint) {
		return fmt.Errorf("expected STS dualstack endpoint %s not found in agent logs", expectedStsEndpoint)
	}

	return nil
}

func checkServiceEndpoints() error {
	cmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log | grep api.aws"
	output, err := common.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to read agent logs: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("no dualstack endpoints found in agent logs")
	}


	for _, expectedEndpoint := range expectedEndpoints {
		if !strings.Contains(output, expectedEndpoint) {
			return fmt.Errorf("expected dualstack endpoint %s not found in agent logs", expectedEndpoint)
		}
	}

	return nil
}
