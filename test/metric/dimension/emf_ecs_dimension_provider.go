// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
)

type EMFECSDimensionProvider struct {
	Provider
}

var _ IProvider = (*EMFECSDimensionProvider)(nil)

func (p EMFECSDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.ECS
}

func (p EMFECSDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key == "Type" {
		return types.Dimension{
			Name:  aws.String("Type"),
			Value: aws.String("Counter"),
		}
	}

	return types.Dimension{}
}

func (p EMFECSDimensionProvider) Name() string {
	return "EMFECSProvider"
}
