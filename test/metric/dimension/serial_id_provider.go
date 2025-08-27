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

type SerialIdDimensionProvider struct {
	Provider
}

var _ IProvider = (*SerialIdDimensionProvider)(nil)

func (p *SerialIdDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *SerialIdDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "SerialId" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	serial, err := common.GetAnyInstanceStoreSerialID()
	if err != nil {
		log.Print(err)
		return types.Dimension{}
	}
	return types.Dimension{
		Name:  aws.String("SerialId"),
		Value: aws.String(serial),
	}
}

func (p *SerialIdDimensionProvider) Name() string {
	return "SerialIdDimensionProvider"
}
