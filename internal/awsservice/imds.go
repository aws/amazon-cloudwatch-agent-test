// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

type imdsSDK struct {
	cxt        context.Context
	imdsClient *imds.Client
}

// GetInstanceId returns the Instance ID of the current instance
func GetInstanceId() (string, error) {
	metadata, err := imdsClient.GetInstanceIdentityDocument(cxt, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return "", err
	}
	return metadata.InstanceID, nil
}
