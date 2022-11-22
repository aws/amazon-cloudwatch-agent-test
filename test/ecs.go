// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package test

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"strings"
)

var (
	ecsCtx    context.Context
	ecsClient *ecs.Client
)

func RestartDaemonService(clusterArn, serviceName string) error {
	return RestartService(clusterArn, nil, serviceName)
}

func RestartService(clusterArn string, desiredCount *int32, serviceName string) error {
	svc, ctx, err := getEcsClient()
	if err != nil {
		return err
	}

	updateServiceInput := &ecs.UpdateServiceInput{
		Cluster:            aws.String(clusterArn),
		Service:            aws.String(serviceName),
		ForceNewDeployment: true,
	}
	if desiredCount != nil {
		updateServiceInput.DesiredCount = desiredCount
	}

	_, err = svc.UpdateService(ctx, updateServiceInput)

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
	svc, ctx, err := getEcsClient()
	if err != nil {
		return nil, err
	}

	input := &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterArn),
	}

	return svc.ListContainerInstances(ctx, input)
}

func describeContainerInstances(clusterArn string, containerInstanceArns []string) (*ecs.DescribeContainerInstancesOutput, error) {
	svc, ctx, err := getEcsClient()
	if err != nil {
		return nil, err
	}

	input := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterArn),
		ContainerInstances: containerInstanceArns,
	}

	return svc.DescribeContainerInstances(ctx, input)
}

func getEcsClient() (*ecs.Client, context.Context, error) {
	if ecsClient == nil {
		ecsCtx = context.Background()
		cfg, err := config.LoadDefaultConfig(ecsCtx)
		if err != nil {
			return nil, nil, err
		}

		ecsClient = ecs.NewFromConfig(cfg)
	}
	return ecsClient, ecsCtx, nil
}
