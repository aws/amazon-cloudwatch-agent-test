// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package old_windows_migration


import (
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"testing"
	"time"
)


const (
	configWindowsJSON               = "resources/config_windows.json"
	metricWindowsnamespace          = "OldWindowsTest"
	configWindowsOutputPath         = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath             = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentWindowsRuntime             = 3 * time.Minute
	numberofWindowsAppendDimensions = 1
)


func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestOldWindowsMigration(t *testing.T){
	// create own configuration and read it
	params := map[string][]string{
		"status": []string{"Enabled"},
		"properties": properties
	}
	awsservice.sendCommand("DocumentName", params)

	time.sleep(agentWindowsRuntime)

	awsservice.sendCommand("DocumentName", params)

	time.sleep(agentWindowsRuntime)


	ok, err := awsservice.ValidateLogs(logGroup, logStream, &startTime, &endTime, func(logs []string) bool {
		if len(logs) < 1 {
			return false
		}
		for _, l := range logs {
			switch logSource {
			case "WindowsEvents":
				if logLevel != "" || !strings.Contains(l, logLine) || !strings.Contains(l, logLevel) {
					return false
				}
			}

		}

		return true
	})

}
