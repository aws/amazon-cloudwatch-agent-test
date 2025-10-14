// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package helper

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type AgentStatus struct {
	Status       string `json:"status"`
	ConfigStatus string `json:"configstatus"`
	Version      string `json:"version"`
	StartTime    string `json:"starttime"`
}

func RunSSMAction(documentName string, instanceIds []string, params map[string][]string, actionName, expectedStatus, expectedConfigStatus string) error {
	log.Printf("Testing %s action", actionName)

	out, err := awsservice.RunSSMDocument(documentName, instanceIds, params)
	if err != nil {
		return fmt.Errorf("%s action failed: %v", actionName, err)
	}

	if err := VerifyAgentActionError(out, instanceIds[0], documentName, expectedStatus, expectedConfigStatus); err != nil {
		return fmt.Errorf("%s verification failed: %v", actionName, err)
	}

	log.Printf("%s action completed successfully", actionName)
	return nil
}

func VerifyAgentActionError(out *ssm.SendCommandOutput, instanceId, documentName, expectedAgentStatus string, expectedConfigStatus string) error {
	var status AgentStatus

	//Wait for command completion
	_, err := awsservice.WaitForCommandCompletion(*out.Command.CommandId, instanceId)
	if err != nil {
		return fmt.Errorf("failed to get command result: %v", err)
	}

	// Verify agent status
	statusParams := map[string][]string{"action": {"status"}}
	statusOut, err := awsservice.RunSSMDocument(documentName, []string{instanceId}, statusParams)
	if err != nil {
		return fmt.Errorf("failed to check agent status: %v", err)
	}

	statusResult, err := awsservice.WaitForCommandCompletion(*statusOut.Command.CommandId, instanceId)
	if err != nil {
		return fmt.Errorf("failed to get status result: %v", err)
	}

	for _, plugin := range statusResult.CommandInvocations[0].CommandPlugins {
		if plugin.Status == types.CommandPluginStatusFailed {
			return fmt.Errorf("command plugin failed: %s", *plugin.Name)
		}
		if plugin.Status == types.CommandPluginStatusTimedOut {
			return fmt.Errorf("command plugin timed out: %s", *plugin.Name)
		}
		outputAsByte := []byte(*plugin.Output)
		if json.Valid(outputAsByte) {
			err := json.Unmarshal([]byte(*plugin.Output), &status)
			if err != nil {
				return fmt.Errorf("failed to unmarshal status output: %v", err)
			}
		}
	}

	if status.Status != expectedAgentStatus {
		return fmt.Errorf("agent status verification failed. Expected: %s, Output: %s", expectedAgentStatus, status.Status)
	}
	if status.ConfigStatus != expectedConfigStatus {
		return fmt.Errorf("config status verification failed. Expected: %s, Output: %s", expectedConfigStatus, status.ConfigStatus)
	}

	return nil
}
