// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

func GetInstanceId() string {
	return GetImdsMetadata().InstanceID
}

func GetImageId() string {
	return GetImdsMetadata().ImageID
}

func GetInstanceType() string {
	return GetImdsMetadata().InstanceType
}

// TODO: Refactor Structure and Interface for more easier follow that shares the same session
func GetImdsMetadata() *imds.GetInstanceIdentityDocumentOutput {
	ctx := context.Background()
	c, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		// fail fast so we don't continue the test
		log.Fatalf("Error occurred while creating SDK config: %v", err)
	}

	// TODO: this only works for EC2 based testing
	client := imds.NewFromConfig(c)
	metadata, err := client.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		log.Fatalf("Error occurred while retrieving EC2 instance ID: %v", err)
	}
	return metadata
}
