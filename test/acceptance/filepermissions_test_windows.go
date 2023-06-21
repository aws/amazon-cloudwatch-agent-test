// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package acceptance

import (
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"go.uber.org/multierr"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

const (
	agentConfigPath    = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Configs\\file_agent_config.json"
	translatedTomlPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.toml"
)

var filePaths = []string{agentConfigPath, translatedTomlPath}

func Validate() error {
	log.Printf("Testing file permissions for windows")
	var multiErr error
	for _, path := range filePaths {
		err := checkFilePermissionsForFilePath(path)
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
			log.Printf("CloudWatchAgent's %s path does not have protection from local system and admin: %v", path, err)
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
