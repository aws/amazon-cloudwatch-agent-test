// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type ecsSDK struct {
	cxt       context.Context
	ecsClient *ecs.Client
}

// RestartDaemonService restarts the service as a daemon by create a new single task and return error if not able to restart it
func RestartDaemonService(clusterArn, serviceName string) error {
	return RestartService(clusterArn, 1, serviceName)
}

// RestartService restarts the service by create desired tasks and return error if not able to restart it
func RestartService(clusterArn string, desiredCount int, serviceName string) error {
	_, err := ecsClient.UpdateService(cxt, &ecs.UpdateServiceInput{
		Cluster:            aws.String(clusterArn),
		Service:            aws.String(serviceName),
		ForceNewDeployment: true,
		DesiredCount:       aws.Int32(int32(desiredCount)),
	})

	return err
}

type ContainerInstance struct {
	ContainerInstanceArn string
	ContainerInstanceId  string
	EC2InstanceId        string
}

// GetContainerInstances gets a list of container ID, container ARN, and EC2 InstanceID given a cluster ARN
func GetContainerInstances(clusterArn string) ([]ContainerInstance, error) {
	containerInstanceArns, err := GetContainerInstanceArns(clusterArn)
	if err != nil {
		return []ContainerInstance{}, err
	}

	describeContainerInstancesOutput, err := ecsClient.DescribeContainerInstances(cxt, &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterArn),
		ContainerInstances: containerInstanceArns,
	})

	if err != nil {
		return []ContainerInstance{}, err
	}

	results := []ContainerInstance{}

	for _, containerInstance := range describeContainerInstancesOutput.ContainerInstances {
		arn := *containerInstance.ContainerInstanceArn
		result := ContainerInstance{
			ContainerInstanceArn: arn,
			ContainerInstanceId:  GetContainerInstanceId(arn),
			EC2InstanceId:        *(containerInstance.Ec2InstanceId),
		}
		results = append(results, result)
	}

	return results, nil
}

// GetContainerInstances gets a list of container ARN given a cluster ARN
func GetContainerInstanceArns(clusterArn string) ([]string, error) {
	listContainerInstancesOutput, err := ecsClient.ListContainerInstances(cxt, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterArn),
	})

	if err != nil {
		return []string{}, err
	}

	return listContainerInstancesOutput.ContainerInstanceArns, nil
}

// GetContainerInstanceId parses container instance ARN and return container instance id
func GetContainerInstanceId(containerInstanceArn string) string {
	return strings.Split(containerInstanceArn, "/")[2]
}

// GetClusterName parses cluster ARN and return cluster name
func GetClusterName(clusterArn string) string {
	return strings.Split(clusterArn, ":cluster/")[1]
}
