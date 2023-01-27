// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type CustomDimensionProvider struct {
	Provider
}

var _ IProvider = (*CustomDimensionProvider)(nil)

func (p *CustomDimensionProvider) IsApplicable() bool {
	return true
}

func (p *CustomDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	if !instruction.Value.IsKnown() {
		return types.Dimension{}
	}

	return types.Dimension{
		Name:  aws.String(instruction.Key),
		Value: instruction.Value.Value,
	}
}

func (p *CustomDimensionProvider) Name() string {
	return "CustomDimensionProvider"
}
