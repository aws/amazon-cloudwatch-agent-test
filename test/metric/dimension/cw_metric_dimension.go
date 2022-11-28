// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func GetExpectedDimensions(env *environment.MetaData, names []string) []types.Dimension {
	dimensions := []types.Dimension{}

	for _, name := range names {
		if name == "InstanceId" {
			dimensions = append(dimensions, getInstanceIdDimension())
		}
	}

	return dimensions
}

func getInstanceIdDimension() types.Dimension {
	ec2InstanceId := test.GetInstanceId()

	//TODO For now they can stay. Later host metrics fetchers might need to be flexible on how to get instance Id
	//because that will be different when testing for ecs ec2 launch type vs plain ec2
	return types.Dimension{
		Name:  aws.String("InstanceId"),
		Value: aws.String(ec2InstanceId),
	}
}
