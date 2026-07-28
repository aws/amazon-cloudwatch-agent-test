// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/google/uuid"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// Validate runs the full SSM Document integration test sequence. Platform-specific
// constants (envConfigPath) and initialization (platformSetup) are defined in the
// build-tagged platform files (ssm_document_unix.go, ssm_document_windows.go).
func Validate() error {
	log.Println("Starting SSM Document validation tests")

	platformSetup()

	// Generate unique ID to guarantee uniqueness
	uniqueID := uuid.New().String()[:8]
	documentName := testManageAgentDocument + uniqueID
	agentConfig1Name := agentConfigFile1 + "-" + uniqueID
	agentConfig2Name := agentConfigFile2 + "-" + uniqueID
	metadata := environment.GetEnvironmentMetaData()
	instanceIds := []string{metadata.InstanceId}

	// Wait for SSM agent to be ready before running tests
	log.Printf("Waiting for instance %s to be SSM-ready...", metadata.InstanceId)
	if err := awsservice.WaitForSSMReady(instanceIds, 5*time.Minute); err != nil {
		return fmt.Errorf("instance not SSM-ready: %v", err)
	}
	log.Println("Instance is SSM-ready")

	log.Printf("Creating SSM document: %s", documentName)
	err := awsservice.CreateSSMDocument(documentName, manageAgentDoc, types.DocumentTypeCommand)
	if err != nil {
		return err
	}

	// Ensure SSM document is cleaned up regardless of test outcome
	defer func() {
		log.Printf("Cleaning up SSM document: %s", documentName)
		if deleteErr := awsservice.DeleteSSMDocument(documentName); deleteErr != nil {
			log.Printf("Warning: Failed to delete SSM document %s: %v", documentName, deleteErr)
		} else {
			log.Printf("Successfully deleted SSM document: %s", documentName)
		}
	}()

	// Test start action
	startTest := testCase{
		parameters:           map[string][]string{paramAction: {actionStart}},
		actionName:           actionStart,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, startTest); err != nil {
		return err
	}

	// Test stop action
	stopTest := testCase{
		parameters:           map[string][]string{paramAction: {actionStop}},
		actionName:           actionStop,
		expectedAgentStatus:  agentStatusStopped,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, stopTest); err != nil {
		return err
	}

	// Test configure (remove) action
	removeTest := testCase{
		parameters: map[string][]string{
			paramAction:                      {actionConfigureRemove},
			paramOptionalConfigurationSource: {configSourceAll},
			paramOptionalRestart:             {restartNo},
		},
		actionName:           actionConfigureRemove,
		expectedAgentStatus:  agentStatusStopped,
		expectedConfigStatus: configStatusNotConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, removeTest); err != nil {
		return err
	}

	// Test configure action
	log.Printf("Putting SSM parameter: %s", agentConfig1Name)
	if err := awsservice.PutStringParameter(agentConfig1Name, agentConfig1); err != nil {
		return err
	}
	defer cleanupSSMParameter(agentConfig1Name)

	configureTest := testCase{
		parameters: map[string][]string{
			paramAction:                        {actionConfigure},
			paramOptionalConfigurationSource:   {configSourceSSM},
			paramOptionalConfigurationLocation: {agentConfig1Name},
		},
		actionName:           actionConfigure,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, configureTest); err != nil {
		return err
	}

	// Test configure (append) action
	log.Printf("Putting SSM parameter: %s", agentConfig2Name)
	if err := awsservice.PutStringParameter(agentConfig2Name, agentConfig2); err != nil {
		return err
	}
	defer cleanupSSMParameter(agentConfig2Name)

	appendTest := testCase{
		parameters: map[string][]string{
			paramAction:                        {actionConfigureAppend},
			paramOptionalConfigurationSource:   {configSourceSSM},
			paramOptionalConfigurationLocation: {agentConfig2Name},
		},
		actionName:           actionConfigureAppend,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, appendTest); err != nil {
		return err
	}

	// Test set-env action (happy path): custom (non translator-managed) key with a value
	// containing spaces. Verifies the ctl's "Set <KEY>" output, that the pair is persisted
	// to env-config.json on disk, and that agent status/configstatus are unchanged.
	setEnvTest := testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvKey1 + "=" + setEnvValue1},
		},
		actionName:           actionSetEnv,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMActionWithOutput(documentName, instanceIds, setEnvTest, setEnvOutputPrefix+setEnvKey1); err != nil {
		return err
	}
	if err := VerifyEnvConfigContent(map[string]string{
		setEnvKey1: setEnvValue1,
	}); err != nil {
		return err
	}

	// Test set-env action (merge): a second key is persisted alongside the first.
	setEnvMergeTest := testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvKey2 + "=" + setEnvValue2},
		},
		actionName:           actionSetEnvMerge,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMActionWithOutput(documentName, instanceIds, setEnvMergeTest, setEnvOutputPrefix+setEnvKey2); err != nil {
		return err
	}
	if err := VerifyEnvConfigContent(map[string]string{
		setEnvKey1: setEnvValue1,
		setEnvKey2: setEnvValue2,
	}); err != nil {
		return err
	}

	// Test set-env action (overwrite): set an existing key to a new value and verify the
	// updated value is persisted (exercises the ctl's MergeFile overwrite-same-key path).
	setEnvOverwriteTest := testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvKey1 + "=" + setEnvOverwriteValue},
		},
		actionName:           actionSetEnvOverwrite,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMActionWithOutput(documentName, instanceIds, setEnvOverwriteTest, setEnvOutputPrefix+setEnvKey1); err != nil {
		return err
	}
	if err := VerifyEnvConfigContent(map[string]string{
		setEnvKey1: setEnvOverwriteValue,
		setEnvKey2: setEnvValue2,
	}); err != nil {
		return err
	}

	// Test set-env action (allowedPattern rejection): a value containing '$' violates the
	// document's optionalEnvironmentVariable allowedPattern and is rejected by SSM at
	// SendCommand time (InvalidParameters error), before the ctl ever runs.
	if err := VerifySSMSendCommandRejection(documentName, instanceIds, testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvInvalidPatternValue},
		},
		actionName: actionSetEnvInvalidPattern,
	}); err != nil {
		return err
	}

	// Test set-env action (allowedPattern rejection): a value containing a backtick violates
	// the allowedPattern and is rejected by SSM at SendCommand time.
	if err := VerifySSMSendCommandRejection(documentName, instanceIds, testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvInvalidBacktickValue},
		},
		actionName: actionSetEnvInvalidBacktick,
	}); err != nil {
		return err
	}

	// Test set-env action (allowedPattern rejection): a key starting with a digit violates
	// the [A-Za-z_] prefix requirement and is rejected by SSM at SendCommand time.
	if err := VerifySSMSendCommandRejection(documentName, instanceIds, testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {setEnvInvalidKeyValue},
		},
		actionName: actionSetEnvInvalidKey,
	}); err != nil {
		return err
	}

	// Test set-env action (error path): empty optionalEnvironmentVariable must fail
	// with the document-level error message.
	// expectedAgentStatus and expectedConfigStatus are intentionally omitted: the action
	// fails at the document level (RunAndVerifySSMActionFailure), so VerifyAgentAction
	// is never called and those fields are unused.
	setEnvEmptyTest := testCase{
		parameters: map[string][]string{
			paramAction:                      {actionSetEnv},
			paramOptionalEnvironmentVariable: {""},
		},
		actionName: actionSetEnvEmpty,
	}
	if err := RunAndVerifySSMActionFailure(documentName, instanceIds, setEnvEmptyTest, setEnvEmptyErrorMessage); err != nil {
		return err
	}

	log.Println("All SSM Document validation tests completed successfully")
	return nil
}
