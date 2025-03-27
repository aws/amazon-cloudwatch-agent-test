// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dimension

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
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
	// TOOD: Assumes nvme0 is the only device used
	serial, err := GetVolumeSerial()
	if err != nil {
		log.Print(err)
		return types.Dimension{}
	}
	return types.Dimension{
		Name:  aws.String("VolumeId"),
		Value: aws.String(serial),
	}
}

func GetVolumeSerial() (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/sys/class/nvme/nvme0/serial"))
	if err != nil {
		return "", nil
	}
	return cleanupString(string(data)), nil
}

func cleanupString(input string) string {
	// Some device info strings use fixed-width padding and/or end with a new line
	return strings.TrimSpace(strings.TrimSuffix(input, "\n"))
}

func (p *VolumeIdDimensionProvider) Name() string {
	return "VolumeIdDimensionProvider"
}
