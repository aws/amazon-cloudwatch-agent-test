// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"os"
)

type HostDimensionProvider struct {
	Provider
}

var _ IProvider = (*HostDimensionProvider)(nil)

func (p *HostDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.EC2
}

func (p *HostDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if instruction.Key != "host" || instruction.Value.IsKnown() {
		return types.Dimension{}
	}
	name, err := os.Hostname()
	if err != nil {
		return types.Dimension{}
	}
	return types.Dimension{
		Name:  aws.String("host"),
		Value: aws.String(name),
	}
}

func (p *HostDimensionProvider) Name() string {
	return "HostDimensionProvider"
}
