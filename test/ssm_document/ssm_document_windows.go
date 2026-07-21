// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package ssm_document

import (
	_ "embed"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/google/uuid"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

var (
	//go:embed resources/test_amazoncloudwatch_manageagent.json
	manageAgentDoc string
	//go:embed resources/agent_config1.json
	agentConfig1 string
	//go:embed resources/agent_config2.json
	agentConfig2 string
)

// set-env on-disk verification: read the agent's env-config.json through the stock
// run-command document for this platform.
const (
	runCommandDocumentName = "AWS-RunPowerShellScript"
	envConfigReadCommand   = `Get-Content -Raw "$Env:ProgramData\Amazon\AmazonCloudWatchAgent\env-config.json"`
)

func Validate() error {
	log.Println("Starting SSM Document validation tests")

	// Generate unique ID to guarantee uniqueness
	uniqueID := uuid.New().String()[:8]
	documentName := testManageAgentDocument + uniqueID
	agentConfig1Name := agentConfigFile1 + "-" + uniqueID
	agentConfig2Name := agentConfigFile2 + "-" + uniqueID
	metadata := environment.GetEnvironmentMetaData()
	instanceIds := []string{metadata.InstanceId}

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
	if err := VerifyEnvConfigContent(runCommandDocumentName, envConfigReadCommand, metadata.InstanceId, map[string]string{
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
	if err := VerifyEnvConfigContent(runCommandDocumentName, envConfigReadCommand, metadata.InstanceId, map[string]string{
		setEnvKey1: setEnvValue1,
		setEnvKey2: setEnvValue2,
	}); err != nil {
		return err
	}

	// Test set-env action (error path): empty optionalEnvironmentVariable must fail
	// with the document-level error message.
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
