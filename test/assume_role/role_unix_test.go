// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	credsDir = "/tmp/.aws"
)

func getCommands(roleArn string) []string {
	return []string{
		"mkdir -p " + credsDir,
		"printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\naws_session_token=%s' $(aws sts assume-role --role-arn " + roleArn + " --role-session-name test --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' --output text) | tee " + credsDir + "/credentials>/dev/null",
		"printf '[default]\nregion = us-west-2' > " + credsDir + "/config",
		"printf '[credentials]\n  shared_credential_profile = \"default\"\n  shared_credential_file = \"" + credsDir + "/credentials\"' | sudo tee /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml>/dev/null",
	}
}

func getDimensions(instanceId string) []types.Dimension {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
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
