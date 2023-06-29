// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package acceptance

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"go.uber.org/multierr"
)

const (
	agentWindowsLogPath   = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentCopiedConfigPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Configs\\file_config.json"
	translatedTomlPath    = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.toml"
)

var filePermissionsPath = []string{agentWindowsLogPath, agentCopiedConfigPath, translatedTomlPath}

func Validate() error {
	log.Printf("testing windows filepermissions")

	var multiErr error
	for _, path := range filePermissionsPath {
		err = checkFilePermissionsForFilePath(path)
		if err != nil {
			log.Printf("CloudWatchAgent's %s does not have protection from local system and admin: %v", path, err)
			multiErr = multierr.Append(multiErr, err)
		}
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
