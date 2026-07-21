// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// cleanupSSMParameter best-effort deletes an SSM parameter, logging any failure. Defer it
// immediately after each PutStringParameter so a failure in a later step never leaves a
// uniquely-named parameter orphaned in the account.
func cleanupSSMParameter(name string) {
	log.Printf("Cleaning up SSM parameter: %s", name)
	if err := awsservice.DeleteParameter(name); err != nil {
		log.Printf("Warning: Failed to delete SSM parameter %s: %v", name, err)
	}
}

func RunAndVerifySSMAction(documentName string, instanceIds []string, tc testCase) error {
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

func VerifyAgentAction(out *ssm.SendCommandOutput, instanceId, documentName string, tc testCase) error {
	var status agentStatus

	//Wait for command completion
	_, err := awsservice.WaitForCommandCompletion(*out.Command.CommandId, instanceId)
	if err != nil {
		// Try to get command output even on failure for better error reporting
		commandOutput := awsservice.GetCommandInvocationDetails(*out.Command.CommandId, instanceId)
		return fmt.Errorf("failed to get command result: %v\nCommand output:\n%s", err, commandOutput)
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

// RunAndVerifySSMActionWithOutput behaves like RunAndVerifySSMAction and additionally
// asserts that expectedOutput appears in the command's output.
func RunAndVerifySSMActionWithOutput(documentName string, instanceIds []string, tc testCase, expectedOutput string) error {
	log.Printf("Testing %s action", tc.actionName)

	out, err := awsservice.RunSSMDocument(documentName, instanceIds, tc.parameters)
	if err != nil {
		return fmt.Errorf("%s action failed: %v", tc.actionName, err)
	}

	result, err := awsservice.WaitForCommandCompletion(*out.Command.CommandId, instanceIds[0])
	if err != nil {
		commandOutput := awsservice.GetCommandInvocationDetails(*out.Command.CommandId, instanceIds[0])
		return fmt.Errorf("%s action failed to complete: %v\nCommand output:\n%s", tc.actionName, err, commandOutput)
	}

	if !commandOutputContains(result, expectedOutput) {
		commandOutput := awsservice.GetCommandInvocationDetails(*out.Command.CommandId, instanceIds[0])
		return fmt.Errorf("%s output verification failed: expected output %q not found\nCommand output:\n%s", tc.actionName, expectedOutput, commandOutput)
	}

	if err := VerifyAgentAction(out, instanceIds[0], documentName, tc); err != nil {
		return fmt.Errorf("%s verification failed: %v", tc.actionName, err)
	}

	log.Printf("%s action completed successfully", tc.actionName)
	return nil
}

// RunAndVerifySSMActionFailure runs the document action and expects the command invocation
// to reach the terminal Failed state (e.g. document-level parameter validation errors).
// If expectedOutput is non-empty, it must appear in the failed command's output.
func RunAndVerifySSMActionFailure(documentName string, instanceIds []string, tc testCase, expectedOutput string) error {
	log.Printf("Testing %s action (expecting failure)", tc.actionName)

	out, err := awsservice.RunSSMDocument(documentName, instanceIds, tc.parameters)
	if err != nil {
		return fmt.Errorf("%s action failed to send: %v", tc.actionName, err)
	}

	commandId := *out.Command.CommandId
	_, err = awsservice.WaitForCommandCompletion(commandId, instanceIds[0])
	commandOutput := awsservice.GetCommandInvocationDetails(commandId, instanceIds[0])
	if err == nil {
		return fmt.Errorf("%s action was expected to fail but succeeded\nCommand output:\n%s", tc.actionName, commandOutput)
	}
	// WaitForCommandCompletion also errors on Cancelled/TimedOut/deadline; require Failed specifically.
	if !strings.Contains(err.Error(), "terminal status "+string(types.CommandInvocationStatusFailed)) {
		return fmt.Errorf("%s action reached an unexpected terminal state: %v\nCommand output:\n%s", tc.actionName, err, commandOutput)
	}
	if expectedOutput != "" && !strings.Contains(commandOutput, expectedOutput) {
		return fmt.Errorf("%s failure output verification failed: expected output %q not found\nCommand output:\n%s", tc.actionName, expectedOutput, commandOutput)
	}

	log.Printf("%s action failed as expected", tc.actionName)
	return nil
}

// VerifyEnvConfigContent reads the agent's env-config.json on the instance by running
// readCommand through runCommandDocument (AWS-RunShellScript or AWS-RunPowerShellScript)
// and asserts that the file contains every expected key/value pair. Additional keys in
// the file are ignored.
func VerifyEnvConfigContent(runCommandDocument, readCommand, instanceId string, expected map[string]string) error {
	log.Printf("Verifying env-config.json content via %s", runCommandDocument)

	out, err := awsservice.RunSSMDocument(runCommandDocument, []string{instanceId}, map[string][]string{"commands": {readCommand}})
	if err != nil {
		return fmt.Errorf("failed to run env-config read command: %v", err)
	}

	result, err := awsservice.WaitForCommandCompletion(*out.Command.CommandId, instanceId)
	if err != nil {
		commandOutput := awsservice.GetCommandInvocationDetails(*out.Command.CommandId, instanceId)
		return fmt.Errorf("env-config read command failed: %v\nCommand output:\n%s", err, commandOutput)
	}

	var content string
	for _, plugin := range result.CommandInvocations[0].CommandPlugins {
		if plugin.Output != nil {
			content += *plugin.Output
		}
	}

	// Extract the JSON object from the raw command output (tolerates surrounding whitespace/noise).
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end < start {
		return fmt.Errorf("no JSON object found in env-config read output:\n%s", content)
	}

	var envConfig map[string]string
	if err := json.Unmarshal([]byte(content[start:end+1]), &envConfig); err != nil {
		return fmt.Errorf("failed to unmarshal env-config.json content: %v\nContent:\n%s", err, content)
	}

	for key, want := range expected {
		got, ok := envConfig[key]
		if !ok {
			return fmt.Errorf("env-config.json is missing expected key %q. Content: %v", key, envConfig)
		}
		if got != want {
			return fmt.Errorf("env-config.json key %q verification failed. Expected: %q, Got: %q", key, want, got)
		}
	}

	log.Println("env-config.json content verified successfully")
	return nil
}

// commandOutputContains reports whether any command plugin's output contains expected.
func commandOutputContains(result *ssm.ListCommandInvocationsOutput, expected string) bool {
	for _, plugin := range result.CommandInvocations[0].CommandPlugins {
		if plugin.Output != nil && strings.Contains(*plugin.Output, expected) {
			return true
		}
	}
	return false
}
