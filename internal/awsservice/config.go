// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package awsservice

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
)

var (
	AWS = NewAWSServiceAPI()
)

type awsServiceAPI struct {
	cxt context.Context

	Ec2API   ec2API
	EcsAPI   ecsAPI
	CwmAPI   cwmAPI
	CwlAPI   cwlAPI
	SsmAPI   ssmAPI
	ImdsAPI  imdsAPI
	DnmdbAPI dnmdbAPI
}

func NewAWSServiceAPI() *awsServiceAPI {
	cxt := context.Background()
	awsCfg, err := config.LoadDefaultConfig(cxt)

	if err != nil {
		log.Fatalf("")
	}

	return &awsServiceAPI{
		cxt:      cxt,
		Ec2API:   NewEC2SDKClient(awsCfg, cxt),
		EcsAPI:   NewECSSDKClient(awsCfg, cxt),
		CwmAPI:   NewCloudWatchSDKClient(awsCfg, cxt),
		CwlAPI:   NewCloudWatchLogsSDKClient(awsCfg, cxt),
		SsmAPI:   NewSSMSDKClient(awsCfg, cxt),
		ImdsAPI:  NewIMDSSDKClient(awsCfg, cxt),
		DnmdbAPI: NewDynamoDBSDKClient(awsCfg, cxt),
	}
}
