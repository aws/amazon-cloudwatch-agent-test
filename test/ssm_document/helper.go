// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"

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

func runAndVerifySSMAction(documentName string, instanceIds []string, tc testCase) error {
	log.Printf("Testing %s action", tc.actionName)

	out, err := awsservice.RunSSMDocument(documentName, instanceIds, tc.parameters)
	if err != nil {
		return fmt.Errorf("%s action failed: %v", tc.actionName, err)
	}

	if err := verifyAgentAction(out, instanceIds[0], documentName, tc); err != nil {
		return fmt.Errorf("%s verification failed: %v", tc.actionName, err)
	}

	log.Printf("%s action completed successfully", tc.actionName)
	return nil
}

func verifyAgentAction(out *ssm.SendCommandOutput, instanceId, documentName string, tc testCase) error {
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

	if len(statusResult.CommandInvocations) == 0 {
		return fmt.Errorf("no command invocations returned for status check")
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

// runAndVerifySSMActionWithOutput behaves like runAndVerifySSMAction and additionally
// asserts that expectedOutput appears in the command's output.
func runAndVerifySSMActionWithOutput(documentName string, instanceIds []string, tc testCase, expectedOutput string) error {
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

	if err := verifyAgentAction(out, instanceIds[0], documentName, tc); err != nil {
		return fmt.Errorf("%s verification failed: %v", tc.actionName, err)
	}

	log.Printf("%s action completed successfully", tc.actionName)
	return nil
}

// runAndVerifySSMActionFailure runs the document action and expects the command invocation
// to reach the terminal Failed state (e.g. document-level parameter validation errors).
// If expectedOutput is non-empty, it must appear in the failed command's output.
func runAndVerifySSMActionFailure(documentName string, instanceIds []string, tc testCase, expectedOutput string) error {
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
	// WaitForCommandCompletion returns a *CommandTerminalError for Failed/Cancelled/TimedOut;
	// require specifically the Failed status.
	var termErr *awsservice.CommandTerminalError
	if !errors.As(err, &termErr) || termErr.Status != types.CommandInvocationStatusFailed {
		return fmt.Errorf("%s action reached an unexpected terminal state: %v\nCommand output:\n%s", tc.actionName, err, commandOutput)
	}
	if expectedOutput != "" && !strings.Contains(commandOutput, expectedOutput) {
		return fmt.Errorf("%s failure output verification failed: expected output %q not found\nCommand output:\n%s", tc.actionName, expectedOutput, commandOutput)
	}

	log.Printf("%s action failed as expected", tc.actionName)
	return nil
}

// verifyEnvConfigContent reads the agent's env-config.json directly from the local
// filesystem (the test runs on the instance) and asserts that the file contains every
// expected key/value pair. Additional keys in the file are ignored.
func verifyEnvConfigContent(expected map[string]string) error {
	log.Printf("Verifying env-config.json content via direct file read: %s", envConfigPath)

	data, err := os.ReadFile(envConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read env-config.json at %s: %v", envConfigPath, err)
	}

	var envConfig map[string]string
	if err := json.Unmarshal(data, &envConfig); err != nil {
		return fmt.Errorf("failed to unmarshal env-config.json: %v\nContent:\n%s", err, string(data))
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
	if len(result.CommandInvocations) == 0 {
		return false
	}
	for _, plugin := range result.CommandInvocations[0].CommandPlugins {
		if plugin.Output != nil && strings.Contains(*plugin.Output, expected) {
			return true
		}
	}
	return false
}

// verifySSMSendCommandRejection runs the document action and expects SendCommand itself
// to reject the request (e.g. because a parameter value violates an allowedPattern).
// The AWS SDK returns an InvalidParameters error from SendCommand in this case.
func verifySSMSendCommandRejection(documentName string, instanceIds []string, tc testCase) error {
	log.Printf("Testing %s action (expecting SendCommand rejection)", tc.actionName)

	_, err := awsservice.RunSSMDocument(documentName, instanceIds, tc.parameters)
	if err == nil {
		return fmt.Errorf("%s action was expected to be rejected at SendCommand but succeeded", tc.actionName)
	}

	// SSM returns an InvalidParameters API error when allowedPattern validation fails.
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) || apiErr.ErrorCode() != "InvalidParameters" {
		return fmt.Errorf("%s action failed with unexpected error (expected InvalidParameters API error): %v", tc.actionName, err)
	}

	log.Printf("%s action rejected at SendCommand as expected: %v", tc.actionName, err)
	return nil
}
