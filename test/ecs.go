// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package test

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"strings"
)

func RestartDaemonService(clusterArn *string, serviceName *string) error {
	return RestartService(clusterArn, nil, serviceName)
}

func RestartService(clusterArn *string, desiredCount *int64, serviceName *string) error {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ecs.New(sess)

	forceNewDeployment := true

	updateServiceInput := &ecs.UpdateServiceInput{
		Cluster:            clusterArn,
		Service:            serviceName,
		ForceNewDeployment: &forceNewDeployment,
	}
	if desiredCount != nil {
		updateServiceInput = updateServiceInput.SetDesiredCount(*desiredCount)
	}

	_, err := svc.UpdateService(updateServiceInput)

	return err
}

type ContainerInstance struct {
	ContainerInstanceArn string
	ContainerInstanceId  string
	EC2InstanceId        string
}

func GetContainerInstances(clusterArn *string) ([]ContainerInstance, error) {
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
			ContainerInstanceId:  GetContainerInstanceId(arn),
			EC2InstanceId:        *(containerInstance.Ec2InstanceId),
		}
		results = append(results, result)
	}

	return results, nil
}

func GetContainerInstanceArns(clusterArn *string) ([]*string, error) {
	listContainerInstancesOutput, err := listContainerInstances(clusterArn)
	if err != nil {
		return []*string{}, err
	}

	return listContainerInstancesOutput.ContainerInstanceArns, nil
}

func GetContainerInstanceId(containerInstanceArn *string) string {
	return strings.Split(*containerInstanceArn, "/")[2]
}

func GetClusterName(clusterArn *string) string {
	return strings.Split(*clusterArn, ":cluster/")[1]
}

func listContainerInstances(clusterArn *string) (*ecs.ListContainerInstancesOutput, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ecs.New(sess)

	input := &ecs.ListContainerInstancesInput{
		Cluster: clusterArn,
	}

	return svc.ListContainerInstances(input)
}

func describeContainerInstances(clusterArn *string, containerInstanceArns []*string) (*ecs.DescribeContainerInstancesOutput, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ecs.New(sess)

	input := &ecs.DescribeContainerInstancesInput{
		Cluster:            clusterArn,
		ContainerInstances: containerInstanceArns,
	}

	return svc.DescribeContainerInstances(input)
}
