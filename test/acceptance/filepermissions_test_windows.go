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

func Validate() error {
	log.Printf("Testing file permissions for windows")
	var multiErr error
	err := checkFilePermissionsForFilePath(agentConfigPath)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
		log.Printf("CloudWatchAgent's config path does not have protection from local system and admin: %v", err)
	}
	err = checkFilePermissionsForFilePath(translatedTomlPath)
	if err != nil {
		multiErr = multierr.Append(multiErr, err)
		log.Printf("CloudWatchAgent's toml path does not have protection from local system and admin: %v", err)
	}
	/*err = common.DeleteFile(agentConfigPath)
	if err != nil {
		log.Printf("Failed to delete config file; err=%v\n", err)
		multiErr = multierr.Append(multiErr, err)
	}*/
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