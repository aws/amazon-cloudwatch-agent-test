// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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

	region := os.Getenv("AWS_REGION")
	if region == "" {
		// default to us-west-2
		region = "us-west-2"
	}

	err := ConfigureAWSClients(region)
	if err != nil {
		fmt.Println("There was an error trying to configure the AWS clients: ", err)
	}
}

// ConfigureAWSClients configures the AWS clients using a set region.
func ConfigureAWSClients(region string) error {
	mu.Lock()
	defer mu.Unlock()

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled))
	if err != nil {
		// handle error
		fmt.Println("There was an error trying to load default config: ", err)
		return err
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

	return nil
}
