// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package proxy

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const commonConfigPath = "/opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"

func GetCommandToCreateProxyConfig(proxyUrl string) []string {
	return []string{
		"echo [proxy] | sudo tee -a /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml",
		"echo http_proxy = \\\"" + proxyUrl + "\\\" | sudo tee -a " + commonConfigPath,
		"echo no_proxy = \\\"169.254.169.254\\\" | sudo tee -a " + commonConfigPath,
	}
}

func getDimensions(instanceId string) []types.Dimension {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "cpu",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")},
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}
