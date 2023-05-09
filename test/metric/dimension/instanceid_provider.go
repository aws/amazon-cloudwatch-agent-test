// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"log"
)

type ECSInstanceIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*ECSInstanceIdDimensionProvider)(nil)

func (p *ECSInstanceIdDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.ECS
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

func (p *ECSInstanceIdDimensionProvider) Name() string {
	return "ECSInstanceIdDimensionProvider"
}

type LocalInstanceIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*LocalInstanceIdDimensionProvider)(nil)

func (p *LocalInstanceIdDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *LocalInstanceIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "InstanceId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	ec2InstanceId := awsservice.GetInstanceId()
	return types.Dimension{
		Name:  aws.String("InstanceId"),
		Value: aws.String(ec2InstanceId),
	}
}

func (p *LocalInstanceIdDimensionProvider) Name() string {
	return "LocalInstanceIdDimensionProvider"
}

type EKSClusterNameProvider struct {
	Provider
}

func (p *EKSClusterNameProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EKS
}

func (p *EKSClusterNameProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "ClusterName" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}

	return types.Dimension{
		Name:  aws.String("ClusterName"),
		Value: aws.String(p.env.EKSClusterName),
	}
}

func (p *EKSClusterNameProvider) Name() string {
	return "EKSClusterNameProvider"
}

var _ IProvider = (*EKSClusterNameProvider)(nil)
