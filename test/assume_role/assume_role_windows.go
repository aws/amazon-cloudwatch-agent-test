// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package assume_role

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	credsDirWindows                 = "C:\\Users\\Admin\\.aws"
	dataDir                         = "${Env:ProgramData}\\Amazon\\AmazonCloudWatchAgent"
	metricWindowsnamespace          = "NvidiaGPUWindowsTest"
	configWindowsJSON               = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\agent_config.json"
	configWindowsOutputPath         = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\config.json"
	agentWindowsLogPath             = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
	agentWindowsRuntime             = 3 * time.Minute
	numberofWindowsAppendDimensions = 1
)

var (
	expectedNvidiaGPUWindowsMetrics = []string{"Memory % Committed Bytes In Use", "nvidia_smi utilization_gpu", "nvidia_smi utilization_memory", "nvidia_smi power_draw", "nvidia_smi temperature_gpu"}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func Validate(assumeRoleArn string) error {
	err := common.CopyFile(configWindowsJSON, configWindowsOutputPath)
	if err != nil {
		log.Printf("Copying agent config file failed: %v", err)
		return err
	}

	err = common.RunCommands(getCommandsWindows(assumeRoleArn))
	if err != nil {
		log.Printf("Creating credentials file failed: %v", err)
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

	dimensionFilter := awsservice.BuildDimensionFilterList(numberofWindowsAppendDimensions)
	for _, metricName := range expectedNvidiaGPUWindowsMetrics {
		err = awsservice.ValidateMetric(metricName, metricWindowsnamespace, dimensionFilter)
		if err != nil {
			log.Printf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
			return err
		}
	}

	err = filesystem.CheckFileRights(agentWindowsLogPath)
	if err != nil {
		log.Printf("CloudWatchAgent's log does not have protection from local system and admin: %v", err)
		return err
	}

	return nil
}

func getCommandsWindows(roleArn string) []string {
	return []string{
		"new-item -itemtype directory -path \"" + credsDirWindows + "\"",
		"$Creds = (Use-STSRole -RoleArn \"" + roleArn + "\" -RoleSessionName \"test\").Credentials",
		"Write-Output \"[default]\" | Set-Content -Path \"" + credsDirWindows + "\\credentials\"",
		"Write-Output \"aws_access_key_id = $Creds.AccessKeyId\" | Set-Content -Append -Path \"" + credsDirWindows + "\\credentials\"",
		"Write-Output \"aws_secret_access_key = $Creds.SecretAccessKey\" | Set-Content -Append -Path \"" + credsDirWindows + "\\credentials\"",
		"Write-Output \"aws_session_token = $Creds.SessionToken\" | Set-Content -Append -Path \"" + credsDirWindows + "\\credentials\"",
		"Write-Output \"aws_session_token = $Creds.SessionToken\" | Set-Content -Append -Path \"" + credsDirWindows + "\\credentials\"",
		"Write-Output \"[default]\" | Set-Content -Path \"" + credsDirWindows + "\\config\"",
		"Write-Output \"region = us-west-2\" | Set-Content -Path \"" + credsDirWindows + "\\config\"",
		"Write-Output \"[credentials]\" | Set-Content -Path \"" + dataDir + "\\common-config.toml\"",
		"Write-Output \"  shared_credential_profile = \"\"default\"\"\" | Set-Content -Append -Path \"" + dataDir + "\\common-config.toml\"",
		"Write-Output \"  shared_credential_file = \"\"" + credsDirWindows + "\\credentials\"\"\" | Set-Content -Append -Path \"" + dataDir + "\\common-config.toml\"",
	}
}

func getDimensionsWindows(instanceId string) []types.Dimension {
	return []types.Dimension{
		types.Dimension{Name: aws.String("InstanceId"), Value: aws.String(instanceId)},
		types.Dimension{Name: aws.String("cpu"), Value: aws.String("cpu-total")},
	}
}
