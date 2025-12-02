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

func Validate() error {
	log.Println("Starting SSM Document validation tests")

	// Generate unique ID to guarantee uniqueness
	uniqueID := uuid.New().String()[:8]
	documentName := testManageAgentDocument + uniqueID
	metadata := environment.GetEnvironmentMetaData()
	instanceIds := []string{metadata.InstanceId}

	// Wait for SSM agent to be ready before running tests
	log.Printf("Waiting for SSM agent to be ready on instance %s", metadata.InstanceId)
	if err := awsservice.WaitForSSMAgentReady(metadata.InstanceId); err != nil {
		return err
	}
	log.Println("SSM agent is ready")

	log.Printf("Creating SSM document: %s", documentName)
	err := awsservice.CreateSSMDocument(documentName, manageAgentDoc, types.DocumentTypeCommand)
	if err != nil {
		return err
	}

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
	log.Printf("Putting SSM parameter: %s", agentConfigFile1)
	if err := awsservice.PutStringParameter(agentConfigFile1, agentConfig1); err != nil {
		return err
	}
	configureTest := testCase{
		parameters: map[string][]string{
			paramAction:                        {actionConfigure},
			paramOptionalConfigurationSource:   {configSourceSSM},
			paramOptionalConfigurationLocation: {agentConfigFile1},
		},
		actionName:           actionConfigure,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, configureTest); err != nil {
		return err
	}

	// Test configure (append) action
	log.Printf("Putting SSM parameter: %s", agentConfigFile2)
	if err := awsservice.PutStringParameter(agentConfigFile2, agentConfig2); err != nil {
		return err
	}
	appendTest := testCase{
		parameters: map[string][]string{
			paramAction:                        {actionConfigureAppend},
			paramOptionalConfigurationSource:   {configSourceSSM},
			paramOptionalConfigurationLocation: {agentConfigFile2},
		},
		actionName:           actionConfigureAppend,
		expectedAgentStatus:  agentStatusRunning,
		expectedConfigStatus: configStatusConfigured,
	}
	if err := RunAndVerifySSMAction(documentName, instanceIds, appendTest); err != nil {
		return err
	}

	log.Printf("Deleting SSM document: %s", documentName)
	err = awsservice.DeleteSSMDocument(documentName)
	if err != nil {
		return err
	}

	log.Println("All SSM Document validation tests completed successfully")
	return nil
}
