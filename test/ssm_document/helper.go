// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

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

	out, _, err := awsservice.RunSSMDocumentAwaitDelivery(documentName, instanceIds, tc.parameters)
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
	statusOut, statusResult, err := awsservice.RunSSMDocumentAwaitDelivery(documentName, []string{instanceId}, statusParams)
	if err != nil {
		return fmt.Errorf("failed to get status result: %v", err)
	}

	if len(statusResult.CommandInvocations) == 0 {
		return fmt.Errorf("no command invocations returned for status check")
	}

	// Same populate-after-Success race as the output assertions: an empty plugin Output
	// yields a zero-value agentStatus and a misleading "Expected: running, Output: "
	// mismatch. Re-read until a plugin emits parseable JSON.
	statusCommandId := *statusOut.Command.CommandId
	for attempt := 0; attempt <= outputPollAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(outputPollInterval)
			refreshed, refreshErr := awsservice.GetCommandInvocationResult(statusCommandId, instanceId)
			if refreshErr != nil {
				log.Printf("Re-reading status command %s failed (attempt %d/%d): %v", statusCommandId, attempt, outputPollAttempts, refreshErr)
				continue
			}
			if len(refreshed.CommandInvocations) == 0 {
				continue
			}
			statusResult = refreshed
		}

		parsed := false
		for _, plugin := range statusResult.CommandInvocations[0].CommandPlugins {
			if plugin.Status == types.CommandPluginStatusFailed {
				return fmt.Errorf("command plugin failed: %s", *plugin.Name)
			}
			if plugin.Status == types.CommandPluginStatusTimedOut {
				return fmt.Errorf("command plugin timed out: %s", *plugin.Name)
			}
			if plugin.Output == nil || strings.TrimSpace(*plugin.Output) == "" {
				continue
			}
			outputAsByte := []byte(*plugin.Output)
			if !json.Valid(outputAsByte) {
				continue
			}
			// Unmarshal into a fresh value: json.Unmarshal merges into a non-zero
			// destination, so reusing `status` would let one plugin (or an earlier
			// attempt) contribute fields to a struct that never existed as a whole.
			var candidate agentStatus
			if err := json.Unmarshal(outputAsByte, &candidate); err != nil {
				return fmt.Errorf("failed to unmarshal status output: %v", err)
			}
			status = candidate
			parsed = true
			break
		}
		if parsed && status.Status != "" {
			if attempt > 0 {
				log.Printf("Agent status for command %s parsed on re-read attempt %d/%d", statusCommandId, attempt, outputPollAttempts)
			}
			break
		}
	}

	if status.Status == "" {
		return fmt.Errorf("agent status output for command %s never populated after %d re-reads over %s\nCommand output:\n%s",
			statusCommandId, outputPollAttempts, time.Duration(outputPollAttempts)*outputPollInterval,
			awsservice.GetCommandInvocationDetails(statusCommandId, instanceId))
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

	out, result, err := awsservice.RunSSMDocumentAwaitDelivery(documentName, instanceIds, tc.parameters)
	if err != nil {
		commandOutput := "no command was sent"
		if out != nil {
			commandOutput = awsservice.GetCommandInvocationDetails(*out.Command.CommandId, instanceIds[0])
		}
		return fmt.Errorf("%s action failed to complete: %v\nCommand output:\n%s", tc.actionName, err, commandOutput)
	}

	if ok, inspected := commandOutputContainsWithRetry(result, *out.Command.CommandId, instanceIds[0], expectedOutput); !ok {
		return fmt.Errorf("%s output verification failed: expected output %q not found\nCommand output:\n%s", tc.actionName, expectedOutput, inspected)
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

	out, _, err := awsservice.RunSSMDocumentAwaitDelivery(documentName, instanceIds, tc.parameters)
	if out == nil {
		return fmt.Errorf("%s action failed to send: %v", tc.actionName, err)
	}

	commandId := *out.Command.CommandId
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
	if expectedOutput != "" {
		refreshed, ok := commandDetailsContainWithRetry(commandId, instanceIds[0], expectedOutput, commandOutput)
		if !ok {
			return fmt.Errorf("%s failure output verification failed: expected output %q not found\nCommand output:\n%s", tc.actionName, expectedOutput, refreshed)
		}
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
// SSM makes a command invocation's terminal Status visible before it populates
// CommandPlugins[].Output, so an assertion made on the result WaitForCommandCompletion
// returned can see Status=Success with an empty Output and wrongly conclude the expected
// text is absent. The helpers below re-read a bounded number of times, but ONLY while the
// output is still unpopulated: once any plugin has emitted output, a missing expected
// string is a real failure and is reported immediately. That keeps the retry from masking
// a genuinely wrong output, and keeps genuine failures fast.
const (
	outputPollAttempts = 6
	outputPollInterval = 5 * time.Second
)

func outputPollBackoff() time.Duration {
	return outputPollInterval + time.Duration(rand.Int63n(int64(time.Second)))
}

// commandOutputPopulated reports whether any plugin has emitted non-empty output yet.
func commandOutputPopulated(result *ssm.ListCommandInvocationsOutput) bool {
	if len(result.CommandInvocations) == 0 {
		return false
	}
	for _, plugin := range result.CommandInvocations[0].CommandPlugins {
		if plugin.Output != nil && strings.TrimSpace(*plugin.Output) != "" {
			return true
		}
	}
	return false
}

// commandOutputContainsWithRetry checks the already-fetched result first, then re-reads
// while the output is unpopulated. It returns the rendering of whatever it last inspected
// so the caller reports the same snapshot the decision was made on.
func commandOutputContainsWithRetry(result *ssm.ListCommandInvocationsOutput, commandId, instanceId, expected string) (bool, string) {
	if commandOutputContains(result, expected) {
		return true, ""
	}
	if commandOutputPopulated(result) {
		return false, awsservice.GetCommandInvocationDetails(commandId, instanceId)
	}
	for attempt := 1; attempt <= outputPollAttempts; attempt++ {
		time.Sleep(outputPollBackoff())
		refreshed, err := awsservice.GetCommandInvocationResult(commandId, instanceId)
		if err != nil {
			log.Printf("Re-reading command %s output failed (attempt %d/%d): %v", commandId, attempt, outputPollAttempts, err)
			continue
		}
		if commandOutputContains(refreshed, expected) {
			log.Printf("Expected output for command %s appeared on re-read attempt %d/%d", commandId, attempt, outputPollAttempts)
			return true, ""
		}
		if commandOutputPopulated(refreshed) {
			return false, awsservice.GetCommandInvocationDetails(commandId, instanceId)
		}
	}
	return false, fmt.Sprintf("output never populated after %d re-reads over %s\n%s",
		outputPollAttempts, time.Duration(outputPollAttempts)*outputPollInterval,
		awsservice.GetCommandInvocationDetails(commandId, instanceId))
}

// commandDetailsContainWithRetry is the string-rendered equivalent for failure paths.
// It returns the most recent rendering so the caller can report what was actually seen.
func commandDetailsContainWithRetry(commandId, instanceId, expected, details string) (string, bool) {
	if strings.Contains(details, expected) {
		return details, true
	}
	for attempt := 1; attempt <= outputPollAttempts; attempt++ {
		time.Sleep(outputPollBackoff())
		details = awsservice.GetCommandInvocationDetails(commandId, instanceId)
		if strings.Contains(details, expected) {
			log.Printf("Expected failure output for command %s appeared on re-read attempt %d/%d", commandId, attempt, outputPollAttempts)
			return details, true
		}
	}
	return details, false
}

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
