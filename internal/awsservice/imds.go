// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

type imdsAPI interface {
	// GetInstanceId returns the Instance ID of the current instance
	GetInstanceId() (string, error)
}

type imdsConfig struct {
	cxt        context.Context
	imdsClient *imds.Client
}

func NewIMDSConfig(cfg aws.Config, cxt context.Context) imdsAPI {
	imdsClient := imds.NewFromConfig(cfg)
	return &imdsConfig{
		cxt:        cxt,
		imdsClient: imdsClient,
	}
}

// GetInstanceId returns the Instance ID of the current instance
func (i *imdsConfig) GetInstanceId() (string, error) {
	metadata, err := i.imdsClient.GetInstanceIdentityDocument(i.cxt, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return "", err
	}
	return metadata.InstanceID, nil
}
