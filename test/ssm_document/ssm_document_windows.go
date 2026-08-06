// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package ssm_document

import (
	_ "embed"
)

var (
	//go:embed resources/test_amazoncloudwatch_manageagent.json
	manageAgentDoc string
	//go:embed resources/agent_config1.json
	agentConfig1 string
	//go:embed resources/agent_config2.json
	agentConfig2 string
)

// envConfigPath is the on-disk path to the agent's env-config.json on Windows.
const envConfigPath = `C:\ProgramData\Amazon\AmazonCloudWatchAgent\env-config.json`

// platformSetup performs platform-specific initialization (no-op on Windows).
func platformSetup() {}
