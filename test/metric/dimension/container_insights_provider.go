// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"log"
)

type ContainerInsightsDimensionProvider struct {
	Provider
}

var _ IProvider = (*ContainerInsightsDimensionProvider)(nil)

func (p *ContainerInsightsDimensionProvider) IsApplicable() bool {
	return p.Provider.env.ComputeType == computetype.ECS
}

func (p *ContainerInsightsDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key == "ClusterName" {
		return types.Dimension{
			Name:  aws.String("ClusterName"),
			Value: aws.String(p.Provider.env.EcsClusterName),
		}
	}

	if instruction.Key == "ContainerInstanceId" {
		//TODO currently assuming there's only one container
		containerInstances, err := awsservice.GetContainerInstances(p.Provider.env.EcsClusterArn)
		if err != nil {
			log.Print(err)
			return types.Dimension{}
		}

		return types.Dimension{
			Name:  aws.String("ContainerInstanceId"),
			Value: aws.String(containerInstances[0].ContainerInstanceId),
		}
	}

	return types.Dimension{}
}

func (p *ContainerInsightsDimensionProvider) Name() string {
	return "ContainerInsightsDimensionProvider"
}
