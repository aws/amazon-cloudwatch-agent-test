// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package restart

func Validate() error {
	return LogCheck("cwa_log=$(if [ -f \"/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log\" ]; then cat \"/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log\" | wc -l; else echo 0; fi) && echo \"cwa_log:${cwa_log}\"")
}
