// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package test

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func GetInstancePrivateIpDns(instanceId string) (*string, error) {
	instanceData, err := DescribeInstances([]string(&instanceId))
	if err != nil {
		return err
	}

	return instanceData.Reservations.Instances[0].PrivateDnsName
}

func DescribeInstances(intanceIds []*string) (*ec2.DescribeInstancesOutput, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ec2.New(sess)

	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}

	return svc.DescribeInstances(describeInstancesInput)
}
