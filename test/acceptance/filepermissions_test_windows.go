// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build windows

package acceptance

import (
	"go.uber.org/multierr"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

const (
	agentRuntime         = 1 * time.Second
	agentConfigLocalPath = "agent_configs/minimum_config.json"
	agentConfigPath      = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Configs\\file_amazon-cloudwatch-agent.json"
	translatedTomlPath   = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.toml"
)

func TestFilePermissions() error {
	log.Printf("Testing file permissions for windows")
	var multiErr error
	common.CopyFile(agentConfigLocalPath, agentConfigPath)
	err := common.StartAgent(agentConfigPath, false, false)
	if err != nil {
		log.Printf("Agent failed to start due to err=%v\n", err)
		return err
	}
	time.Sleep(agentRuntime)
	common.StopAgent()
	err = checkFilePermissionsForFilePath(agentConfigPath)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
	}
	err = checkFilePermissionsForFilePath(translatedTomlPath)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
	}
	err = common.DeleteFile(agentConfigPath)
	if err != nil {
		log.Printf("Failed to delete config file; err=%v\n", err)
		multiErr = multierr.Append(multiErr, err)
	}
	return multiErr
}

func checkFilePermissionsForFilePath(filepath string) error {
	log.Printf("validating file permissions for filepath=%v", filepath)

	err := filesystem.CheckFileRights(filepath)
	if err != nil {
		return err
	}
	log.Printf("SUCCESS: file %s have permission to Local system and administrator", filepath)
	return nil
}
