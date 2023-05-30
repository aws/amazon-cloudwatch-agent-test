// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package proxy

import (
	"fmt"
)

func GetCommandToCreateProxyConfig(proxyUrl string) []string {
	return []string{
		fmt.Sprintf("cat<<EOF\n[proxy]\n  http_proxy = \"%s\"\n  no_proxy = \"169.254.169.254\"\nEOF > sudo tee /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml", proxyUrl),
	}
}
