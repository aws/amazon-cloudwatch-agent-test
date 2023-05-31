// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package acceptance

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
	"time"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

const (
	agentRuntime         = 1 * time.Second
	agentConfigLocalPath = "agent_configs/minimum_config.json"
	agentConfigPath      = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentConfigCopiedDir = "/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.d"
	translatedTomlPath   = "/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml"
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
	testResults := make([]status.TestResult, 3)
	testGroupResult := status.TestGroupResult{
		Name:        "FilePermissions",
		TestResults: testResults,
	}
	testResults[0] = onlyUserCanWriteToFilepath("root", agentConfigPath)
	testResults[1] = onlyUserCanWriteToFilepath("cwagent", agentConfigCopiedDir)
	testResults[2] = onlyUserCanWriteToFilepath("cwagent", translatedTomlPath)
	return testGroupResult
}

func onlyUserCanWriteToFilepath(user, filepath string) status.TestResult {
	log.Printf("validating only user=%v can write to filepath=%v", user, filepath)
	testResult := status.TestResult{
		Name:   filepath,
		Status: status.FAILED,
	}

	hasExpectedOwner, err := fileHasOwnerAndGroup(filepath, user, user)
	if err != nil || !hasExpectedOwner {
		return testResult
	}

	hasExpectedPermission, err := fileHasPermission(filepath, filesystem.OwnerWrite, true)
	if err != nil || !hasExpectedPermission {
		return testResult
	}

	hasExpectedPermission, err = fileHasPermission(filepath, filesystem.AnyoneWrite, false)
	if err != nil || !hasExpectedPermission {
		return testResult
	}
	log.Printf("SUCCESS: only user=%v can write to filepath=%v", user, filepath)
	testResult.Status = status.SUCCESSFUL
	return testResult
}

func fileHasOwnerAndGroup(filepath, expectedOwner, expectedGroup string) (bool, error) {
	owner, err := filesystem.GetFileOwnerUserName(filepath)
	if err != nil {
		err := fmt.Errorf("FAILED fileHasOwnerAndGroup(); error=%w", err)
		log.Println(err)
		return false, err
	} else if owner != expectedOwner {
		log.Printf("FAILED fileHasOwnerAndGroup(); expected OWNER=%v; actual=%v", expectedGroup, owner)
		return false, nil
	}

	group, err := filesystem.GetFileGroupName(filepath)
	if err != nil {
		err := fmt.Errorf("FAILED fileHasOwnerAndGroup(); error=%w", err)
		log.Println(err)
		return false, err
	} else if group != expectedGroup {
		log.Printf("FAILED fileHasOwnerAndGroup(); expected GROUP=%v; actual=%v", expectedGroup, group)
		return false, nil
	}
	return true, nil
}

func fileHasPermission(filepath string, permission filesystem.FilePermission, shouldHavePermission bool) (bool, error) {
	hasPermission, err := filesystem.FileHasPermission(filepath, permission)
	if err != nil {
		err := fmt.Errorf("fileHasPermission(), could not invoke filesystem.FileHasPermission; err=%w", err)
		log.Println(err)
		return false, err
	}
	return hasPermission == shouldHavePermission, nil
}
