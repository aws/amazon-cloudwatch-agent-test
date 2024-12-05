// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	mu  sync.Mutex
	ctx context.Context

	// AWS Clients
	Ec2Client            *ec2.Client
	EcsClient            *ecs.Client
	SsmClient            *ssm.Client
	ImdsClient           *imds.Client
	CwmClient            *cloudwatch.Client
	CwlClient            *cloudwatchlogs.Client
	DynamodbClient       *dynamodb.Client
	S3Client             *s3.Client
	CloudformationClient *cloudformation.Client
	XrayClient           *xray.Client
)

func init() {
	ctx = context.Background()
	var err error
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	if err != nil {
		// handle error
		fmt.Println("There was an error trying to load default config: ", err)
		return
	}
	fmt.Println("This is the aws region: ", awsCfg.Region)

	// Initialize AWS Clients with the configured awsCfg
	Ec2Client = ec2.NewFromConfig(awsCfg)
	EcsClient = ecs.NewFromConfig(awsCfg)
	SsmClient = ssm.NewFromConfig(awsCfg)
	ImdsClient = imds.NewFromConfig(awsCfg)
	CwmClient = cloudwatch.NewFromConfig(awsCfg)
	CwlClient = cloudwatchlogs.NewFromConfig(awsCfg)
	DynamodbClient = dynamodb.NewFromConfig(awsCfg)
	S3Client = s3.NewFromConfig(awsCfg)
	CloudformationClient = cloudformation.NewFromConfig(awsCfg)
	XrayClient = xray.NewFromConfig(awsCfg)
}

// ReconfigureAWSClients reconfigures the AWS clients using a new region.
func ReconfigureAWSClients(region string) error {
	mu.Lock()
	defer mu.Unlock()

	newCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return err
	}

	Ec2Client = ec2.NewFromConfig(newCfg)
	EcsClient = ecs.NewFromConfig(newCfg)
	SsmClient = ssm.NewFromConfig(newCfg)
	ImdsClient = imds.NewFromConfig(newCfg)
	CwmClient = cloudwatch.NewFromConfig(newCfg)
	CwlClient = cloudwatchlogs.NewFromConfig(newCfg)
	DynamodbClient = dynamodb.NewFromConfig(newCfg)
	S3Client = s3.NewFromConfig(newCfg)
	CloudformationClient = cloudformation.NewFromConfig(newCfg)
	XrayClient = xray.NewFromConfig(newCfg)

	return nil
}
