// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/xray"
	backoff "github.com/cenkalti/backoff/v4"
	"time"
)

const (
	StandardRetries = 3
)

var (
	StandardExponentialBackoff = backoff.WithMaxRetries(&backoff.ExponentialBackOff{
		InitialInterval:     30 * time.Second,
		RandomizationFactor: 2,
		Multiplier:          2,
		MaxInterval:         60 * time.Second,
		MaxElapsedTime:      30 * time.Second,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}, StandardRetries)
)

var (
	ctx                  = context.Background()
	awsCfg, _            = config.LoadDefaultConfig(ctx)
	Ec2Client            = ec2.NewFromConfig(awsCfg)
	EcsClient            = ecs.NewFromConfig(awsCfg)
	SsmClient            = ssm.NewFromConfig(awsCfg)
	ImdsClient           = imds.NewFromConfig(awsCfg)
	CwmClient            = cloudwatch.NewFromConfig(awsCfg)
	CwlClient            = cloudwatchlogs.NewFromConfig(awsCfg)
	DynamodbClient       = dynamodb.NewFromConfig(awsCfg)
	S3Client             = s3.NewFromConfig(awsCfg)
	CloudformationClient = cloudformation.NewFromConfig(awsCfg)
	XrayClient           = xray.NewFromConfig(awsCfg)
)
