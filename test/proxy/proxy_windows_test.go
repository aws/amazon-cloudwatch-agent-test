// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package proxy

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func GetCommandToCreateProxyConfig(proxyUrl string) []string {
	return []string{
		fmt.Sprintf("echo '\n[proxy]\n  http_proxy = \\\"%s\\\"\n  no_proxy = \\\"169.254.169.254\\\"' | Set-Content -Path \"%s\"", proxyUrl, "${Env:ProgramData}\\Amazon\\AmazonCloudWatchAgent\\common-config.toml"),
	}
}

func getDimensions(instanceId string) []types.Dimension {
	return []types.Dimension{
		types.Dimension{Name: aws.String("InstanceId"), Value: aws.String(instanceId)},
		types.Dimension{Name: aws.String("cpu"), Value: aws.String("cpu-total")},
	}
}
