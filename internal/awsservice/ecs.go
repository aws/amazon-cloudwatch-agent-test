// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type ecsAPI interface {
	//RestartDaemonService restarts the service as a daemon by create a new single task and return error if not able to restart it
	RestartDaemonService(clusterArn, serviceName string) error

	//RestartService restarts the service by create desired tasks and return error if not able to restart it
	RestartService(clusterArn string, desiredCount int, serviceName string) error

	//GetContainerInstances gets a list of container ID, container ARN, and EC2 InstanceID given a cluster ARN
	GetContainerInstances(clusterArn string) ([]ContainerInstance, error)

	//GetContainerInstances gets a list of container ARN given a cluster ARN
	GetContainerInstanceArns(clusterArn string) ([]string, error)

	//GetContainerInstanceId parses container instance ARN and return container instance id
	GetContainerInstanceId(containerInstanceArn string) string

	//GetClusterName parses cluster ARN and return cluster name
	GetClusterName(clusterArn string) string
}

type ecsConfig struct {
	cxt       context.Context
	ecsClient *ecs.Client
}

func NewECSConfig(cfg aws.Config, cxt context.Context) ecsAPI {
	ecsClient := ecs.NewFromConfig(cfg)
	return &ecsConfig{
		cxt:       cxt,
		ecsClient: ecsClient,
	}
}

// RestartDaemonService restarts the service as a daemon by create a new single task and return error if not able to restart it
func (e *ecsConfig) RestartDaemonService(clusterArn, serviceName string) error {
	return e.RestartService(clusterArn, 1, serviceName)
}

// RestartService restarts the service by create desired tasks and return error if not able to restart it
func (e *ecsConfig) RestartService(clusterArn string, desiredCount int, serviceName string) error {
	_, err := e.ecsClient.UpdateService(e.cxt, &ecs.UpdateServiceInput{
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
func (e *ecsConfig) GetContainerInstances(clusterArn string) ([]ContainerInstance, error) {
	containerInstanceArns, err := e.GetContainerInstanceArns(clusterArn)
	if err != nil {
		return []ContainerInstance{}, err
	}

	describeContainerInstancesOutput, err := e.ecsClient.DescribeContainerInstances(e.cxt, &ecs.DescribeContainerInstancesInput{
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
			ContainerInstanceId:  e.GetContainerInstanceId(arn),
			EC2InstanceId:        *(containerInstance.Ec2InstanceId),
		}
		results = append(results, result)
	}

	return results, nil
}

// GetContainerInstances gets a list of container ARN given a cluster ARN
func (e *ecsConfig) GetContainerInstanceArns(clusterArn string) ([]string, error) {
	listContainerInstancesOutput, err := e.ecsClient.ListContainerInstances(e.cxt, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterArn),
	})

	if err != nil {
		return []string{}, err
	}

	return listContainerInstancesOutput.ContainerInstanceArns, nil
}

// GetContainerInstanceId parses container instance ARN and return container instance id
func (e *ecsConfig) GetContainerInstanceId(containerInstanceArn string) string {
	return strings.Split(containerInstanceArn, "/")[2]
}

// GetClusterName parses cluster ARN and return cluster name
func (e *ecsConfig) GetClusterName(clusterArn string) string {
	return strings.Split(clusterArn, ":cluster/")[1]
}
