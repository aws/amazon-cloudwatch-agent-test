// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

<<<<<<< HEAD:internal/aws/imds.go
//go:build integration
// +build integration

package aws

import (
	"context"
	"log"

=======
package test

import (
	"context"
>>>>>>> main:test/util.go
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"log"
)

// TODO: Refactor Structure and Interface for more easier follow that shares the same session
func GetInstanceId() string {
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
	return metadata.InstanceID
}
