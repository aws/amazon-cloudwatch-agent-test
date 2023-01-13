// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
)

var (
	awsService = NewAWSServiceAPI()
)

type awsServiceAPI struct {
	ec2API  ec2API
	ecsAPI  ecsAPI
	cwAPI   cwAPI
	cwlAPI  cwlAPI
	ssmAPI  ssmAPI
	imdsAPI imdsAPI
}

func NewAWSServiceAPI() *awsServiceAPI {
	cxt := context.Background()
	awsCfg, err := config.LoadDefaultConfig(cxt)

	if err != nil {
		log.Fatalf("")
	}

	return &awsConfig{
		cxt:     cxt,
		ec2API:  NewEc2Config(awsCfg, cxt),
		ecsAPI:  NewECSConfig(awsCfg, cxt),
		cwAPI:   NewCloudWatchConfig(awsCfg, cxt),
		cwlAPI:  NewCloudWatchLOgsConfig(awsCfg, cxt),
		ssmAPI:  NewSSMConfig(awsCfg, cxt),
		imdsAPI: NewIMDSConfig(awsCfg, cxt),
	}
}
