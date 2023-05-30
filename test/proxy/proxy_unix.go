// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package proxy

const commonConfigPath = "/opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"

func GetCommandToCreateProxyConfig(proxyUrl string) []string {
	return []string{
		"echo [proxy] | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
		"echo http_proxy = \\\"" + proxyUrl + "\\\" | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
		"echo no_proxy = \\\"169.254.169.254\\\" | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
	}
}
