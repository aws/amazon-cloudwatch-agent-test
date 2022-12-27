// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
)

type ECSInstanceIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*ECSInstanceIdDimensionProvider)(nil)

func (p *ECSInstanceIdDimensionProvider) IsApplicable() bool {
	if (p.env.ComputeType == computetype.ECS) {
		return true
	}
	return false
}

func (p *ECSInstanceIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "InstanceId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}

	//TODO currently assuming there's only one container
	containerInstances, err := awsservice.GetContainerInstances(p.env.EcsClusterArn)
	if err != nil {
		log.Print(err)
		return types.Dimension{}
	}

	return types.Dimension{
		Name:  aws.String("InstanceId"),
		Value: aws.String(containerInstances[0].EC2InstanceId),
	}
}

type InstanceIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*ECSInstanceIdDimensionProvider)(nil)

func (p *InstanceIdDimensionProvider) IsApplicable() bool {
	if (p.env.ComputeType == computetype.EC2) {
		return true
	}
	return false
}

func (p *InstanceIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "InstanceId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	ec2InstanceId := awsservice.GetInstanceId()
	return types.Dimension{
		Name:  aws.String("InstanceId"),
		Value: aws.String(ec2InstanceId),
	}
}