// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// PutStringParameter add a string parameter to the system. (e.g adding a CWA configuration to SSM)
func PutStringParameter(name, value string) error {
	_, err := SsmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true),
	})

	return err
}
