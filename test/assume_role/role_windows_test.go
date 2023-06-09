// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package assume_role

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	credsDir = "C:\\Users\\Admin\\.aws"
	dataDir  = "${Env:ProgramData}\\Amazon\\AmazonCloudWatchAgent"
)

func getCommands(roleArn string) []string {
	return []string{
		"new-item -itemtype directory -path \"" + credsDir + "\"",
		"$Creds = (Use-STSRole -RoleArn \"" + roleArn + "\" -RoleSessionName \"test\").Credentials",
		"Write-Output \"[default]\" | Set-Content -Path \"" + credsDir + "\\credentials\"",
		"Write-Output \"aws_access_key_id = $Creds.AccessKeyId\" | Set-Content -Append -Path \"" + credsDir + "\\credentials\"",
		"Write-Output \"aws_secret_access_key = $Creds.SecretAccessKey\" | Set-Content -Append -Path \"" + credsDir + "\\credentials\"",
		"Write-Output \"aws_session_token = $Creds.SessionToken\" | Set-Content -Append -Path \"" + credsDir + "\\credentials\"",
		"Write-Output \"aws_session_token = $Creds.SessionToken\" | Set-Content -Append -Path \"" + credsDir + "\\credentials\"",
		"Write-Output \"[default]\" | Set-Content -Path \"" + credsDir + "\\config\"",
		"Write-Output \"region = us-west-2\" | Set-Content -Path \"" + credsDir + "\\config\"",
		"Write-Output \"[credentials]\" | Set-Content -Path \"" + dataDir + "\\common-config.toml\"",
		"Write-Output \"  shared_credential_profile = \"\"default\"\"\" | Set-Content -Append -Path \"" + dataDir + "\\common-config.toml\"",
		"Write-Output \"  shared_credential_file = \"\"" + credsDir + "\\credentials\"\"\" | Set-Content -Append -Path \"" + dataDir + "\\common-config.toml\"",
	}
}

func getDimensions(instanceId string) []types.Dimension {
	return []types.Dimension{
		types.Dimension{Name: aws.String("InstanceId"), Value: aws.String(instanceId)},
		types.Dimension{Name: aws.String("cpu"), Value: aws.String("cpu-total")},
	}
}
