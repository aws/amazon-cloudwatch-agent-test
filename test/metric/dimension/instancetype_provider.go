// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type LocalInstanceTypeDimensionProvider struct {
	Provider
}

var _ IProvider = (*LocalInstanceTypeDimensionProvider)(nil)

func (p *LocalInstanceTypeDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *LocalInstanceTypeDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "InstanceType" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	ec2InstanceType := awsservice.GetInstanceType()
	return types.Dimension{
		Name:  aws.String("InstanceType"),
		Value: aws.String(ec2InstanceType),
	}
}

func (p *LocalInstanceTypeDimensionProvider) Name() string {
	return "LocalInstanceTypeDimensionProvider"
}
