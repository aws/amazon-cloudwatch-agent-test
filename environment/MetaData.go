// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package environment

import (
	"flag"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/compute_type"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecs_deployment_type"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecs_launch_type"
	"github.com/aws/amazon-cloudwatch-agent-test/test"
)

type MetaData struct {
	ComputeType               compute_type.ComputeType
	EcsLaunchType             ecs_launch_type.ECSLaunchType
	EcsDeploymentStrategy     ecs_deployment_type.ECSDeploymentType
	EcsClusterArn             string
	EcsClusterName            string
	CwagentConfigSsmParamName string
	EcsServiceName            string
}

type MetaDataStrings struct {
	ComputeType               string
	EcsLaunchType             string
	EcsDeploymentStrategy     string
	EcsClusterArn             string
	CwagentConfigSsmParamName string
	EcsServiceName            string
}

func registerComputeType(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.ComputeType), "computeType", " default ", "EC2/ECS/EKS")
}
func registerECSData(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.EcsLaunchType), "ecsLaunchType", "", "EC2 or Fargate")
	flag.StringVar(&(dataString.EcsDeploymentStrategy), "ecsDeploymentStrategy", "", "Daemon/Replica/Sidecar")
	flag.StringVar(&(dataString.EcsClusterArn), "clusterArn", "", "Used to restart ecs task to apply new agent config")
	flag.StringVar(&(dataString.CwagentConfigSsmParamName), "cwagentConfigSsmParamName", "", "Used to set new cwa config")
	flag.StringVar(&(dataString.EcsServiceName), "cwagentECSServiceName", "", "Used to restart ecs task to apply new agent config")
}

func fillComputeType(e *MetaData, data *MetaDataStrings) *MetaData {
	computeType, ok := compute_type.FromString(data.ComputeType)
	if !ok {
		panic("Invalid compute type " + data.ComputeType)
	}
	e.ComputeType = computeType
	return e
}

func fillECSData(e *MetaData, data *MetaDataStrings) *MetaData {
	if e.ComputeType != compute_type.ECS {
		return e
	}

	ecsLaunchType, ok := ecs_launch_type.FromString(data.EcsLaunchType)
	if !ok {
		panic("Invalid compute type " + data.ComputeType)
	}
	e.EcsLaunchType = ecsLaunchType

	ecsDeploymentStrategy, ok := ecs_deployment_type.FromString(data.EcsDeploymentStrategy)
	if !ok {
		panic("Invalid compute type " + data.ComputeType)
	}
	e.EcsDeploymentStrategy = ecsDeploymentStrategy

	e.EcsClusterArn = data.EcsClusterArn
	e.CwagentConfigSsmParamName = data.CwagentConfigSsmParamName
	e.EcsServiceName = data.EcsServiceName
	e.EcsClusterName = test.GetClusterName(&(data.EcsClusterArn))

	return e
}

func RegisterEnvironmentMetaDataFlags(metaDataStrings *MetaDataStrings) *MetaDataStrings {
	registerComputeType(metaDataStrings)
	registerECSData(metaDataStrings)
	return metaDataStrings
}

func GetEnvironmentMetaData(data *MetaDataStrings) *MetaData {
	metaData := &(MetaData{})
	metaData = fillComputeType(metaData, data)
	metaData = fillECSData(metaData, data)

	return metaData
}
