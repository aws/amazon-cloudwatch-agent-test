// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build windows

package acceptance

import (
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/stretchr/testify/assert"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

const (
	agentRuntime         = 1 * time.Second
	agentConfigLocalPath = "agent_configs/minimum_config.json"
	agentConfigPath      = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Configs\\file_amazon-cloudwatch-agent.json"
	agentConfigCopiedDir = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.d"
	translatedTomlPath   = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.toml"
)

func TestFilePermissions(t *testing.T) {
	common.CopyFile(agentConfigLocalPath, agentConfigPath)
	err := common.StartAgent(agentConfigPath, false, false)
	if err != nil {
		log.Printf("Agent failed to start due to err=%v\n", err)
	}
	time.Sleep(agentRuntime)
	common.StopAgent()
	groupResult := createGroupResult()
	for _, testResult := range groupResult.TestResults {
		assert.Equal(t, testResult.Status, status.SUCCESSFUL)
	}
	err = common.DeleteFile(agentConfigPath)
	if err != nil {
		log.Printf("Failed to delete config file; err=%v\n", err)
	}
}

func createGroupResult() status.TestGroupResult {
	testResults := make([]status.TestResult, 2)
	testGroupResult := status.TestGroupResult{
		Name:        "FilePermissions",
		TestResults: testResults,
	}
	testResults[0] = checkFilePermissionsForFilePath(agentConfigPath)
	testResults[1] = checkFilePermissionsForFilePath(translatedTomlPath)
	return testGroupResult
}

func checkFilePermissionsForFilePath(filepath string) status.TestResult {
	log.Printf("validating file permissions for filepath=%v", filepath)
	testResult := status.TestResult{
		Name:   filepath,
		Status: status.FAILED,
	}

	err := filesystem.CheckFileRights(filepath)
	if err != nil {
		return testResult
	}
	log.Printf("SUCCESS: file %s have permission to Local system and administrator", filepath)
	testResult.Status = status.SUCCESSFUL
	return testResult
}
