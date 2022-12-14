// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"os"
)

type HostDimensionProvider struct {
	Provider
}

var _ IProvider = (*HostDimensionProvider)(nil)

func (p *HostDimensionProvider) IsApplicable() bool {
	if (p.env.ComputeType == computetype.EC2) {
		return true
	}
	return false
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