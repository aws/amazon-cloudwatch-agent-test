// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

var (
	ec2ctx    context.Context
	ec2Client *ec2.Client
)

func GetInstancePrivateIpDns(instanceId string) (*string, error) {
	instanceData, err := DescribeInstances([]string{instanceId})
	if err != nil {
		return nil, err
	}

	return instanceData.Reservations[0].Instances[0].PrivateDnsName, nil
}

func DescribeInstances(instanceIds []string) (*ec2.DescribeInstancesOutput, error) {
	svc, ctx, err := getEc2Client()
	if err != nil {
		return nil, err
	}

	return svc.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	})
}

func getEc2Client() (*ec2.Client, context.Context, error) {
	if ec2Client == nil {
		ec2ctx = context.Background()
		cfg, err := config.LoadDefaultConfig(ec2ctx)
		if err != nil {
			return nil, nil, err
		}

		ec2Client = ec2.NewFromConfig(cfg)
	}
	return ec2Client, ec2ctx, nil
}
