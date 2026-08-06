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
	// AKS is an Azure Kubernetes Service cluster authenticating to AWS via the projected
	// service-account web-identity credential chain.
	AKS ComputeType = "AKS"
)

var (
	computeTypes = map[string]ComputeType{
		"EC2":     EC2,
		"ECS":     ECS,
		"EKS":     EKS,
		"AZUREVM": AzureVM,
		"AKS":     AKS,
	}
)

func FromString(str string) (ComputeType, bool) {
	c, ok := computeTypes[strings.ToUpper(str)]
	return c, ok
}
