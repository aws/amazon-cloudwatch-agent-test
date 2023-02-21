// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func RestartDaemonService(clusterArn, serviceName string) error {
	return RestartService(clusterArn, nil, serviceName)
}

func RestartService(clusterArn string, desiredCount *int32, serviceName string) error {
	updateServiceInput := &ecs.UpdateServiceInput{
		Cluster:            aws.String(clusterArn),
		Service:            aws.String(serviceName),
		ForceNewDeployment: true,
	}
	if desiredCount != nil {
		updateServiceInput.DesiredCount = desiredCount
	}

	_, err := EcsClient.UpdateService(ctx, updateServiceInput)

	return err
}

type ContainerInstance struct {
	ContainerInstanceArn string
	ContainerInstanceId  string
	EC2InstanceId        string
}

func GetContainerInstances(clusterArn string) ([]ContainerInstance, error) {
	containerInstanceArns, err := GetContainerInstanceArns(clusterArn)
	if err != nil {
		return []ContainerInstance{}, err
	}

	describeContainerInstancesOutput, err := describeContainerInstances(clusterArn, containerInstanceArns)
	if err != nil {
		return []ContainerInstance{}, err
	}

	results := []ContainerInstance{}
	for _, containerInstance := range describeContainerInstancesOutput.ContainerInstances {
		arn := containerInstance.ContainerInstanceArn
		result := ContainerInstance{
			ContainerInstanceArn: *arn,
			ContainerInstanceId:  GetContainerInstanceId(*arn),
			EC2InstanceId:        *(containerInstance.Ec2InstanceId),
		}
		results = append(results, result)
	}

	return results, nil
}

func GetContainerInstanceArns(clusterArn string) ([]string, error) {
	listContainerInstancesOutput, err := listContainerInstances(clusterArn)
	if err != nil {
		return []string{}, err
	}

	return listContainerInstancesOutput.ContainerInstanceArns, nil
}

func GetContainerInstanceId(containerInstanceArn string) string {
	return strings.Split(containerInstanceArn, "/")[2]
}

func GetClusterName(clusterArn string) string {
	return strings.Split(clusterArn, ":cluster/")[1]
}

func listContainerInstances(clusterArn string) (*ecs.ListContainerInstancesOutput, error) {
	return EcsClient.ListContainerInstances(ctx, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterArn),
	})
}

func describeContainerInstances(clusterArn string, containerInstanceArns []string) (*ecs.DescribeContainerInstancesOutput, error) {
	return EcsClient.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterArn),
		ContainerInstances: containerInstanceArns,
	})
}
