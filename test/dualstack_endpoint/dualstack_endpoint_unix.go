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
	agentRuntime     = 3 * time.Minute
	stsAgentRuntime  = 1 * time.Minute
)

var expectedEndpoints = []string{
	"ec2.us-west-2.api.aws",
	"logs.us-west-2.api.aws",
	"monitoring.us-west-2.api.aws",
}

var expectedStsEndpoint = "sts.us-west-2.api.aws"

func ValidateDualstackEndpoints() error {
	log.Printf("Starting dualstack endpoint validation test")

	// First test: Check STS endpoint with role_arn configuration
	err := testStsEndpoint()
	if err != nil {
		return fmt.Errorf("STS endpoint test failed: %w", err)
	}

	// Second test: Check other AWS service endpoints
	err = testServiceEndpoints()
	if err != nil {
		return fmt.Errorf("service endpoints test failed: %w", err)
	}

	log.Printf("All dualstack endpoint tests passed successfully")
	return nil
}

func testStsEndpoint() error {
	log.Printf("Testing STS dualstack endpoint")

	// Copy STS configuration
	common.CopyFile(stsConfigPath, configOutputPath)

	// Start the agent
	err := common.StartAgent(configOutputPath, false, false)
	if err != nil {
		log.Printf("Agent failed to start for STS test: %v", err)
		return fmt.Errorf("failed to start agent for STS test: %w", err)
	}

	// Wait for agent to attempt STS call and generate logs
	log.Printf("Waiting %v for agent to attempt STS call", stsAgentRuntime)
	time.Sleep(stsAgentRuntime)

	// Stop the agent
	common.StopAgent()

	// Check for STS endpoint in agent logs
	return checkStsEndpoint()
}

func testServiceEndpoints() error {
	log.Printf("Testing AWS service dualstack endpoints")

	// Copy regular agent configuration
	common.CopyFile(agentConfigPath, configOutputPath)

	// Start the agent
	err := common.StartAgent(configOutputPath, false, false)
	if err != nil {
		log.Printf("Agent failed to start for service endpoints test: %v", err)
		return fmt.Errorf("failed to start agent for service endpoints test: %w", err)
	}

	// Wait for agent to initialize and generate logs
	log.Printf("Waiting %v for agent to initialize and generate endpoint logs", agentRuntime)
	time.Sleep(agentRuntime)

	// Stop the agent
	common.StopAgent()

	// Check for service endpoints in agent logs
	return checkServiceEndpoints()
}

func checkStsEndpoint() error {
	log.Printf("Checking agent logs for STS dualstack endpoint")

	// Debug: Show full log file content first
	debugCmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	debugOutput, debugErr := common.RunCommand(debugCmd)
	if debugErr != nil {
		log.Printf("DEBUG: Failed to read full agent log: %v", debugErr)
	} else {
		log.Printf("DEBUG: Full agent log content:\n%s", debugOutput)
	}

	// Read agent log file and search for api.aws endpoints
	cmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log | grep -a api.aws"
	log.Printf("DEBUG: Running command: %s", cmd)
	output, err := common.RunCommand(cmd)
	log.Printf("DEBUG: Command output length: %d", len(output))
	log.Printf("DEBUG: Command error: %v", err)

	if err != nil {
		log.Printf("Failed to read agent logs: %v", err)
		return fmt.Errorf("failed to read agent logs: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		log.Printf("DEBUG: No api.aws endpoints found in grep output")
		return fmt.Errorf("no dualstack endpoints found in agent logs")
	}

	log.Printf("Found dualstack endpoint entries in logs:\n%s", output)

	// Check for STS endpoint
	if !strings.Contains(output, expectedStsEndpoint) {
		log.Printf("DEBUG: Expected STS endpoint '%s' not found in output", expectedStsEndpoint)
		return fmt.Errorf("expected STS dualstack endpoint %s not found in agent logs", expectedStsEndpoint)
	}

	log.Printf("✓ Found expected STS dualstack endpoint: %s", expectedStsEndpoint)
	return nil
}

func checkServiceEndpoints() error {
	log.Printf("Checking agent logs for AWS service dualstack endpoints")

	// Debug: Show full log file content first
	debugCmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	debugOutput, debugErr := common.RunCommand(debugCmd)
	if debugErr != nil {
		log.Printf("DEBUG: Failed to read full agent log: %v", debugErr)
	} else {
		log.Printf("DEBUG: Full agent log content:\n%s", debugOutput)
	}

	// Read agent log file and search for api.aws endpoints
	cmd := "cat /opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log | grep api.aws"
	log.Printf("DEBUG: Running command: %s", cmd)
	output, err := common.RunCommand(cmd)
	log.Printf("DEBUG: Command output length: %d", len(output))
	log.Printf("DEBUG: Command error: %v", err)

	if err != nil {
		log.Printf("Failed to read agent logs: %v", err)
		return fmt.Errorf("failed to read agent logs: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		log.Printf("DEBUG: No api.aws endpoints found in grep output")
		return fmt.Errorf("no dualstack endpoints found in agent logs")
	}

	log.Printf("Found dualstack endpoint entries in logs:\n%s", output)

	// Validate that all expected service endpoints are present
	for _, expectedEndpoint := range expectedEndpoints {
		if !strings.Contains(output, expectedEndpoint) {
			log.Printf("DEBUG: Expected endpoint '%s' not found in output", expectedEndpoint)
			return fmt.Errorf("expected dualstack endpoint %s not found in agent logs", expectedEndpoint)
		}
		log.Printf("✓ Found expected dualstack endpoint: %s", expectedEndpoint)
	}

	log.Printf("All expected service dualstack endpoints found in agent logs")
	return nil
}
