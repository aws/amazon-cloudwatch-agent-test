// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package computetype

import "strings"

type ComputeType string

const (
	EC2 ComputeType = "EC2"
	ECS ComputeType = "ECS"
	EKS ComputeType = "EKS"
	// AzureVM is a non-AWS host authenticating to AWS via the Azure web-identity credential chain.
	AzureVM ComputeType = "AZUREVM"
)

var (
	computeTypes = map[string]ComputeType{
		"EC2":     EC2,
		"ECS":     ECS,
		"EKS":     EKS,
		"AZUREVM": AzureVM,
	}
)

func FromString(str string) (ComputeType, bool) {
	c, ok := computeTypes[strings.ToUpper(str)]
	return c, ok
}
