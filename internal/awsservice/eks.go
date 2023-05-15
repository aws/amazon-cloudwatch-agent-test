// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	_ "embed"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type EKSInstance struct {
	InstanceName *string
}

type EKSClusterType struct {
	Type string
}

func GetEKSInstances(clusterName string) ([]EKSInstance, error) {

	describeEksInstancesOutput, err := describeEksInstances(clusterName)
	if err != nil {
		return []EKSInstance{}, err
	}

	var results []EKSInstance
	for _, instance := range describeEksInstancesOutput.Reservations[0].Instances {
		results = append(results, EKSInstance{
			InstanceName: instance.PrivateDnsName,
		})
	}

	return results, nil
}

func describeEksInstances(clusterName string) (*ec2.DescribeInstancesOutput, error) {
	return Ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name: aws.String("tag:aws:eks:cluster-name"),
				Values: []string{
					clusterName,
				},
			},
		},
	})
}
