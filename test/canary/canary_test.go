// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	configInputPath          = "resources/canary_config.json"
	configOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	downloadAgentVersionPath = "./CWAGENT_VERSION"
	installAgentVersionPath  = "/opt/aws/amazon-cloudwatch-agent/bin/CWAGENT_VERSION"
	bakeTime                 = 1 * time.Minute
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// This downloads the latest agent, copies the canary config, starts the agent, confirm version is correct, and validates no errors in logs.
func TestCanary(t *testing.T) {
	// Canary set up
	common.CopyFile(configInputPath, configOutputPath)
	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	common.StartAgent(configOutputPath, true)

	// Version validation
	expectedVersion, err := os.ReadFile(downloadAgentVersionPath)
	if err != nil {
		t.Fatalf("Failure reading downloaded version file %s", downloadAgentVersionPath)
	}
	actualVersion, err := os.ReadFile(installAgentVersionPath)
	if err != nil {
		t.Fatalf("Failure reading installed version file %s", installAgentVersionPath)
	}
	assert.Equal(t, string(expectedVersion), string(actualVersion))

	// Check for log errors
	time.Sleep(bakeTime)
	agentLog, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		t.Fatalf("Failure reading agent log file %s", common.AgentLogFile)
	}
	agentLogString := strings.ToLower(string(agentLog))
	if strings.Contains(agentLogString, "e!") || strings.Contains(agentLogString, "error") {
		t.Fatalf("Erros found in agent log %s", agentLogString)
	}
}
