// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

const (
	testManageAgentDocument = "Test-AmazonCloudWatch-ManageAgent-"

	// Actions
	actionStart           = "start"
	actionStop            = "stop"
	actionConfigure       = "configure"
	actionConfigureAppend = "configure (append)"
	actionConfigureRemove = "configure (remove)"

	// Parameters
	paramAction                        = "action"
	paramOptionalConfigurationSource   = "optionalConfigurationSource"
	paramOptionalConfigurationLocation = "optionalConfigurationLocation"
	paramOptionalRestart               = "optionalRestart"

	// Parameter Values
	configSourceSSM = "ssm"
	configSourceAll = "all"
	restartNo       = "no"

	// Agent Status
	agentStatusRunning = "running"
	agentStatusStopped = "stopped"

	// Config Status
	configStatusConfigured    = "configured"
	configStatusNotConfigured = "not configured"

	// SSM ParametersStore Configs
	agentConfigFile1 = "agentConfig1"
	agentConfigFile2 = "agentConfig2"
)

type agentStatus struct {
	Status       string `json:"status"`
	ConfigStatus string `json:"configstatus"`
	Version      string `json:"version"`
	StartTime    string `json:"starttime"`
}

type testCase struct {
	parameters           map[string][]string
	actionName           string
	expectedAgentStatus  string
	expectedConfigStatus string
}
