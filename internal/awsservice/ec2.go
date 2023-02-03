// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// GetInstancePrivateIpDns returns Instance Private IP Address
func GetInstancePrivateIpDns(instanceId string) (string, error) {
	instanceData, err := ec2Client.DescribeInstances(cxt, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	})

	if err != nil {
		return "", err
	}

	return *instanceData.Reservations[0].Instances[0].PrivateDnsName, nil
}
