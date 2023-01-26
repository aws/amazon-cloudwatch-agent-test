// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package environment

import (
	"flag"
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecsdeploymenttype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecslaunchtype"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
)

type MetaData struct {
	ComputeType               computetype.ComputeType
	EcsLaunchType             ecslaunchtype.ECSLaunchType
	EcsDeploymentStrategy     ecsdeploymenttype.ECSDeploymentType
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
	flag.StringVar(&(dataString.ComputeType), "computeType", "", "EC2/ECS/EKS")
}
func registerECSData(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.EcsLaunchType), "ecsLaunchType", "", "EC2 or Fargate")
	flag.StringVar(&(dataString.EcsDeploymentStrategy), "ecsDeploymentStrategy", "", "Daemon/Replica/Sidecar")
	flag.StringVar(&(dataString.EcsClusterArn), "clusterArn", "", "Used to restart ecs task to apply new agent config")
	flag.StringVar(&(dataString.CwagentConfigSsmParamName), "cwagentConfigSsmParamName", "", "Used to set new cwa config")
	flag.StringVar(&(dataString.EcsServiceName), "cwagentECSServiceName", "", "Used to restart ecs task to apply new agent config")
}

func fillComputeType(e *MetaData, data *MetaDataStrings) *MetaData {
	computeType, ok := computetype.FromString(data.ComputeType)
	if !ok {
		log.Panic("Invalid compute type. Needs to be EC2/ECS/EKS. Compute Type is a required flag. :" + data.ComputeType)
	}
	e.ComputeType = computeType
	return e
}

func fillECSData(e *MetaData, data *MetaDataStrings) *MetaData {
	if e.ComputeType != computetype.ECS {
		return e
	}

	ecsLaunchType, ok := ecslaunchtype.FromString(data.EcsLaunchType)
	if !ok {
		log.Printf("Invalid launch type %s. This might be because it wasn't provided for non-ECS tests", data.ComputeType)
	} else {
		e.EcsLaunchType = ecsLaunchType
	}

	ecsDeploymentStrategy, ok := ecsdeploymenttype.FromString(data.EcsDeploymentStrategy)
	if !ok {
		log.Printf("Invalid deployment strategy %s. This might be because it wasn't provided for non-ECS tests", data.ComputeType)
	} else {
		e.EcsDeploymentStrategy = ecsDeploymentStrategy
	}

	e.EcsClusterArn = data.EcsClusterArn
	e.CwagentConfigSsmParamName = data.CwagentConfigSsmParamName
	e.EcsServiceName = data.EcsServiceName
	e.EcsClusterName = awsservice.AWS.EcsAPI.GetClusterName(data.EcsClusterArn)

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
