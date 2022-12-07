// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

var (
	ssmCtx    context.Context
	ssmClient *ssm.Client
)

func PutStringParameter(name, value string) error {
	return putParameter(name, value, types.ParameterTypeString)
}

func putParameter(name, value string, paramType types.ParameterType) error {
	svc, ctx, err := getSsmClient()
	if err != nil {
		return err
	}
	isOverwriteAllowed := true

	_, err = svc.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      paramType,
		Overwrite: &isOverwriteAllowed,
	})

	return err
}

func getSsmClient() (*ssm.Client, context.Context, error) {
	if ssmClient == nil {
		ssmCtx = context.Background()
		cfg, err := config.LoadDefaultConfig(ssmCtx)
		if err != nil {
			return nil, nil, err
		}

		ssmClient = ssm.NewFromConfig(cfg)
	}
	return ssmClient, ssmCtx, nil
}
