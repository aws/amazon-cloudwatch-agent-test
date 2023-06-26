// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type LocalImageIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*LocalImageIdDimensionProvider)(nil)

func (p *LocalImageIdDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *LocalImageIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "ImageId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	ec2ImageId := awsservice.GetImageId()
	return types.Dimension{
		Name:  aws.String("ImageId"),
		Value: aws.String(ec2ImageId),
	}
}

func (p *LocalImageIdDimensionProvider) Name() string {
	return "LocalImageIdDimensionProvider"
}
