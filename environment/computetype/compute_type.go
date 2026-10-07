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
	// GCE is a non-AWS host authenticating to AWS via the GCP web-identity credential chain.
	GCE ComputeType = "GCE"
	// GKE is a Google Kubernetes Engine cluster authenticating to AWS via the projected
	// service-account web-identity credential chain.
	GKE ComputeType = "GKE"
)

var (
	computeTypes = map[string]ComputeType{
		"EC2":     EC2,
		"ECS":     ECS,
		"EKS":     EKS,
		"AZUREVM": AzureVM,
		"AKS":     AKS,
		"GCE":     GCE,
		"GKE":     GKE,
	}
)

func FromString(str string) (ComputeType, bool) {
	c, ok := computeTypes[strings.ToUpper(str)]
	return c, ok
}
