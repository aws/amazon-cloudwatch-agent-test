// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func RunAndVerifySSMAction(documentName string, instanceIds []string, tc TestCase) error {
	log.Printf("Testing %s action", tc.actionName)

	out, err := awsservice.RunSSMDocument(documentName, instanceIds, tc.parameters)
	if err != nil {
		return fmt.Errorf("%s action failed: %v", tc.actionName, err)
	}

	if err := VerifyAgentAction(out, instanceIds[0], documentName, tc); err != nil {
		return fmt.Errorf("%s verification failed: %v", tc.actionName, err)
	}

	log.Printf("%s action completed successfully", tc.actionName)
	return nil
}

func VerifyAgentAction(out *ssm.SendCommandOutput, instanceId, documentName string, tc TestCase) error {
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

	if status.Status != tc.expectedAgentStatus {
		return fmt.Errorf("agent status verification failed. Expected: %s, Output: %s", tc.expectedAgentStatus, status.Status)
	}
	if status.ConfigStatus != tc.expectedConfigStatus {
		return fmt.Errorf("config status verification failed. Expected: %s, Output: %s", tc.expectedConfigStatus, status.ConfigStatus)
	}

	return nil
}
