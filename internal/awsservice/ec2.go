// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func GetInstancePrivateIpDns(instanceId string) (*string, error) {
	instanceData, err := DescribeInstances([]string{instanceId})
	if err != nil {
		return nil, err
	}

	return instanceData.Reservations[0].Instances[0].PrivateDnsName, nil
}

func DescribeInstances(instanceIds []string) (*ec2.DescribeInstancesOutput, error) {
	return Ec2Client.DescribeInstances(cxt, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	})
}
