// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"fmt"
	"log"
	"strings"
	"time"

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

// WaitForServiceStable waits for an ECS service deployment to complete and stabilize
func WaitForServiceStable(clusterArn, serviceName string, timeout time.Duration) error {
	log.Printf("Waiting for ECS service %s to stabilize (timeout: %v)...", serviceName, timeout)

	// Use AWS SDK's built-in waiter for service stability
	waiter := ecs.NewServicesStableWaiter(EcsClient, func(options *ecs.ServicesStableWaiterOptions) {
		options.MinDelay = 15 * time.Second
		options.MaxDelay = 15 * time.Second
	})

	err := waiter.Wait(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterArn),
		Services: []string{serviceName},
	}, timeout)

	if err != nil {
		return fmt.Errorf("timeout or error waiting for service to stabilize: %w", err)
	}

	log.Printf("Service %s is stable", serviceName)
	return nil
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
