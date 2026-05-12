// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
)

const (
	// DefaultAssumeRoleDuration is the default duration for assumed role credentials (1 hour)
	DefaultAssumeRoleDuration = 3600
)

// AssumeRole assumes a role and returns the AssumeRole output
func AssumeRole(roleArn, sessionName string, durationSeconds int32) (*sts.AssumeRoleOutput, error) {
	result, err := StsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int32(durationSeconds),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role %s: %w", roleArn, err)
	}

	if result.Credentials == nil {
		return nil, fmt.Errorf("no credentials returned from AssumeRole")
	}

	return result, nil
}

// GetCredentials assumes a role and returns the credentials object
func GetCredentials(roleArn, sessionName string, durationSeconds int32) (*types.Credentials, error) {
	result, err := AssumeRole(roleArn, sessionName, durationSeconds)
	if err != nil {
		return nil, err
	}

	return result.Credentials, nil
}
