// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ssm_document

const (
	testManageAgentDocument = "Test-AmazonCloudWatch-ManageAgent-"

	// Actions
	actionStart                = "start"
	actionStop                 = "stop"
	actionConfigure            = "configure"
	actionConfigureAppend      = "configure (append)"
	actionConfigureRemove      = "configure (remove)"
	actionSetEnv               = "set-env"
	actionSetEnvMerge          = "set-env (merge)"
	actionSetEnvOverwrite      = "set-env (overwrite)"
	actionSetEnvEmpty          = "set-env (empty)"
	actionSetEnvInvalidPattern = "set-env (invalid pattern)"

	// actionSetEnvInvalidBacktick exercises rejection of a value containing a backtick.
	actionSetEnvInvalidBacktick = "set-env (invalid backtick)"

	// actionSetEnvInvalidKey exercises rejection of a malformed key starting with a digit.
	actionSetEnvInvalidKey = "set-env (invalid key)"

	// Parameters
	paramAction                        = "action"
	paramOptionalConfigurationSource   = "optionalConfigurationSource"
	paramOptionalConfigurationLocation = "optionalConfigurationLocation"
	paramOptionalRestart               = "optionalRestart"
	paramOptionalEnvironmentVariable   = "optionalEnvironmentVariable"

	// set-env test environment variables. Custom (non translator-managed) key names so
	// configure actions and agent restarts do not overwrite them. Values include spaces
	// to exercise quoting through the document -> ctl -> agent binary chain.
	setEnvKey1   = "CWA_TEST_VAR_ONE"
	setEnvValue1 = "value with spaces"
	setEnvKey2   = "CWA_TEST_VAR_TWO"
	setEnvValue2 = "another value"

	// setEnvOverwriteValue is used to overwrite setEnvKey1 with a new value.
	setEnvOverwriteValue = "overwritten value"

	// setEnvInvalidPatternValue contains a '$' which violates the document's
	// optionalEnvironmentVariable allowedPattern and is rejected by SSM at SendCommand.
	setEnvInvalidPatternValue = "CWA_TEST_INVALID=$not_allowed"

	// setEnvInvalidBacktickValue contains a backtick which violates the document's
	// optionalEnvironmentVariable allowedPattern and is rejected by SSM at SendCommand.
	setEnvInvalidBacktickValue = "CWA_TEST_BACKTICK=val`ue"

	// setEnvInvalidKeyValue has a malformed key starting with a digit, violating the
	// allowedPattern's key portion ([A-Za-z_][A-Za-z0-9_]*=...) and rejected at SendCommand.
	setEnvInvalidKeyValue = "1FOO=bar"

	// setEnvOutputPrefix is printed by the ctl on a successful set-env ("Set <KEY>").
	setEnvOutputPrefix = "Set "

	// setEnvEmptyErrorMessage is printed by the SSM document when optionalEnvironmentVariable
	// is empty for the set-env action.
	setEnvEmptyErrorMessage = "optionalEnvironmentVariable must be specified for the set-env action"

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
	parameters map[string][]string
	// actionName is a human-readable test label used only in log messages and error
	// output. It is NOT the SSM action parameter value (which is carried in
	// parameters[paramAction]).
	actionName           string
	expectedAgentStatus  string
	expectedConfigStatus string
}
