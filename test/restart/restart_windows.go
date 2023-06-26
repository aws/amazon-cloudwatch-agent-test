// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package restart

func Validate() error {
	return LogCheck("$cwa_log=if (Test-Path -LiteralPath \"$Env:ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log\") { (Get-Content \"$Env:ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log\").Length } else {\"0\"}; Write-Output \"cwa_log:$cwa_log\"")
}
