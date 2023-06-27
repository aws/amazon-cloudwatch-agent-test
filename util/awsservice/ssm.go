// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func PutStringParameter(name, value string) error {
	return putParameter(name, value, types.ParameterTypeString)
}

func GetStringParameter(name string) string {
	parameter, err := SsmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return "Parameter not found"
	}

	return *parameter.Parameter.Value
}

func putParameter(name, value string, paramType types.ParameterType) error {
	isOverwriteAllowed := true

	_, err := SsmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      paramType,
		Overwrite: &isOverwriteAllowed,
	})

	return err
}
