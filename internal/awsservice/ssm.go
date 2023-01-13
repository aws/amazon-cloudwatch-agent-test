// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type ssmAPI interface {
	// PutStringParameter add a string parameter to the system. (e.g adding a CWA configuration to SSM)
	PutStringParameter(name, value string) error
}

type ssmConfig struct {
	cxt       context.Context
	ssmClient *ssm.Client
}

func NewEc2Config(cfg aws.Config, cxt context.Context) ssmAPI {
	ssmClient := ssm.NewFromConfig(cfg)
	return &ssmConfig{
		cxt:       cxt,
		ssmClient: ssmClient,
	}
}

// PutStringParameter add a string parameter to the system. (e.g adding a CWA configuration to SSM)
func (s *ssmConfig) PutStringParameter(name, value string) error {
	_, err := s.ssmClient.PutParameter(s.cxt, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true),
	})

	return err
}
