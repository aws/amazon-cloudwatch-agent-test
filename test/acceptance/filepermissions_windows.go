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
)

const (
	// agent config json file in Temp dir gets written by terraform
	configWindowsJSON       = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\agent_config.json"
	configWindowsOutputPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath     = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentWindowsRuntime     = 3 * time.Minute
)

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
	err = filesystem.CheckFileRights(agentWindowsLogPath)
	if err != nil {
		log.Printf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
		return err
	}

	return nil
}
