// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type EKSInstance struct {
	InstanceID *string
	InstanceName *string

}

func GetEKSInstances(clusterName string) ([]EKSInstance, error) {

	describeEksInstancesOutput, err := describeEksInstances(clusterName)
	if err != nil {
		return []EKSInstance{}, err
	}

	var results []EKSInstance
	for _, instance := range describeEksInstancesOutput.Reservations[0].Instances {
		results = append(results, EKSInstance{
			InstanceID: instance.InstanceId,
			InstanceName: instance.PrivateDnsName,
		})
	}

	return results, nil
}

func describeEksInstances(clusterName string) (*ec2.DescribeInstancesOutput, error) {
	return Ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput {
		Filters: []types.Filter {
			{
				Name: aws.String("tag:aws:eks:cluster-name"),
				Values: []string {
					clusterName,
				},
			},
		})
	}
}
