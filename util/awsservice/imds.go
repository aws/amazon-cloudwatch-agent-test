// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

const (
	// imdsMaxAttempts bounds retries of the instance identity document lookup.
	imdsMaxAttempts = 5
	// imdsRetryInterval is the pause between attempts.
	imdsRetryInterval = 2 * time.Second
)

var identityDoc *imds.GetInstanceIdentityDocumentOutput

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
	if identityDoc != nil {
		return identityDoc
	}
	var err error

	// Retry before giving up. A single slow IMDS response used to abort the whole
	// job via log.Fatalf before any assertion ran, because callers reach this from
	// the middle of a test rather than from setup.
	// TODO: this only works for EC2 based testing
	for attempt := 1; attempt <= imdsMaxAttempts; attempt++ {
		identityDoc, err = ImdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
		if err == nil {
			return identityDoc
		}
		log.Printf("Attempt %d/%d: could not retrieve imds identityDoc: %v", attempt, imdsMaxAttempts, err)
		if attempt < imdsMaxAttempts {
			time.Sleep(imdsRetryInterval)
		}
	}
	log.Fatalf("Error occurred while retrieving imds identityDoc after %d attempts: %v", imdsMaxAttempts, err)
	return nil
}
