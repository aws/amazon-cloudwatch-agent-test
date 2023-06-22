// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package acceptance

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"go.uber.org/multierr"
)

const (
	// agent config json file in Temp dir gets written by terraform
	configWindowsJSON       = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\agent_config.json"
	configWindowsOutputPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath     = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentCopiedConfigPath   = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Configs\\file_config.json"
	translatedTomlPath      = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.toml"

	agentWindowsRuntime = 3 * time.Minute
)

var filePermissionsPath = []string{agentWindowsLogPath, agentCopiedConfigPath, translatedTomlPath}

func Validate() error {
	log.Printf("testing windows filepermissions")
	err := common.CopyFile(configWindowsJSON, configWindowsOutputPath)
	if err != nil {
		log.Printf("Copying agent config file failed: %v", err)
		return err
	}

	err = common.StartAgent(configWindowsOutputPath, true, false)
	if err != nil {
		log.Printf("Starting agent failed: %v", err)
		return err
	}

	time.Sleep(agentWindowsRuntime)
	log.Printf("Agent has been running for : %s", agentWindowsRuntime.String())

	err = common.StopAgent()
	if err != nil {
		log.Printf("Stopping agent failed: %v", err)
		return err
	}

	var multiErr error
	for _, path := range filePermissionsPath {
		err = checkFilePermissionsForFilePath(path)
		if err != nil {
			log.Printf("CloudWatchAgent's %s does not have protection from local system and admin: %v", path, err)
			multiErr = multierr.Append(multiErr, err)
		}
	}

	err = common.DeleteFile(configWindowsOutputPath)
	if err != nil {
		log.Printf("Failed to delete config file; err=%v\n", err)
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
