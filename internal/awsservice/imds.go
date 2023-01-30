// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

type imdsAPI interface {
	// GetInstanceId returns the Instance ID of the local instance
	GetInstanceId() (string, error)
}

type imdsSDK struct {
	cxt        context.Context
	imdsClient *imds.Client
}

func NewIMDSSDKClient(cfg aws.Config, cxt context.Context) imdsAPI {
	imdsClient := imds.NewFromConfig(cfg)
	return &imdsSDK{
		cxt:        cxt,
		imdsClient: imdsClient,
	}
}

// GetInstanceId returns the Instance ID of the current instance
func (i *imdsSDK) GetInstanceId() (string, error) {
	metadata, err := i.imdsClient.GetInstanceIdentityDocument(i.cxt, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return "", err
	}
	return metadata.InstanceID, nil
}
