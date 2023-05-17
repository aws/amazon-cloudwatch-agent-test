package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type EFMECSDimensionProvider struct {
	Provider
}

func (p EFMECSDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.ECS
}

func (p EFMECSDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	return types.Dimension{}
}

func (p EFMECSDimensionProvider) Name() string {
	return "EMFECSProvider"
}

var _ IProvider = (*EFMECSDimensionProvider)(nil)
