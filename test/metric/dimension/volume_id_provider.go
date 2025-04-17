// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type VolumeIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*VolumeIdDimensionProvider)(nil)

func (p *VolumeIdDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *VolumeIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "VolumeId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	serial, err := common.GetAnyNvmeVolumeID()
	if err != nil {
		log.Print(err)
		return types.Dimension{}
	}
	return types.Dimension{
		Name:  aws.String("VolumeId"),
		Value: aws.String(serial),
	}
}

func (p *VolumeIdDimensionProvider) Name() string {
	return "VolumeIdDimensionProvider"
}
