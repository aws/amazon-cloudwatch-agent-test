// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type ec2API interface {
	// GetInstancePrivateIpDns returns Instance Private IP Address
	GetInstancePrivateIpDns(instanceId string) (string, error)
}

type ec2Config struct {
	cxt       context.Context
	ec2Client *ec2.Client
}

func NewEC2Config(cfg aws.Config, cxt context.Context) ec2API {
	ec2Client := ec2.NewFromConfig(cfg)
	return &ec2Config{
		cxt:       cxt,
		ec2Client: ec2Client,
	}
}

// GetInstancePrivateIpDns returns Instance Private IP Address
func (e *ec2Config) GetInstancePrivateIpDns(instanceId string) (string, error) {
	instanceData, err := e.ec2Client.DescribeInstances(e.cxt, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	})

	if err != nil {
		return "", err
	}

	return *instanceData.Reservations[0].Instances[0].PrivateDnsName, nil
}
