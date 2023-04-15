// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type ContainerInstance struct {
	ContainerInstanceId string
	EC2InstanceId       string
}

func GetContainerInstances(clusterName string) ([]ContainerInstance, error) {
	containerInstances, err := EcsClient.ListContainerInstances(ctx, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	})

	if err != nil {
		return nil, err
	}

	containerInstancesInformation, err := EcsClient.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterName),
		ContainerInstances: containerInstances.ContainerInstanceArns,
	})

	if err != nil {
		return nil, err
	}

	results := []ContainerInstance{}
	for _, containerInstance := range containerInstancesInformation.ContainerInstances {
		results = append(results, ContainerInstance{
			ContainerInstanceId: GetContainerInstanceId(*containerInstance.ContainerInstanceArn),
			EC2InstanceId:       *containerInstance.Ec2InstanceId,
		})
	}

	return results, nil
}

func GetContainerInstanceId(containerInstanceArn string) string {
	return strings.Split(containerInstanceArn, "/")[2]
}
