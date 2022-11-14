package environment

import (
	"flag"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/compute_type"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecs_deployment_type"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecs_launch_type"
)

type MetaData struct {
	ComputeType               compute_type.ComputeType
	EcsLaunchType             ecs_launch_type.ECSLaunchType
	EcsDeploymentStrategy     ecs_deployment_type.ECSDeploymentType
	EcsClusterArn             string
	CwagentConfigSsmParamName string
	EcsServiceName            string
}

func fillComputeType(flags *flag.FlagSet, e *MetaData) *MetaData {
	computeType, ok := compute_type.FromString(*(flags.String("computeType", "", "EC2/ECS/EKS")))
	if !ok {
		panic("Compute Type not provided")
	}
	e.ComputeType = computeType
	return e
}

func fillECSData(flags *flag.FlagSet, e *MetaData) *MetaData {
	if e.ComputeType != compute_type.ECS {
		return e
	}

	launchType, ok := ecs_launch_type.FromString(*(flags.String("ecsLaunchType", "", "EC2 or Fargate")))
	if !ok {
		panic("Launch Type not provided")
	}
	e.EcsLaunchType = launchType

	deploymentStrategy, ok := ecs_deployment_type.FromString(*(flags.String("ecsDeploymentStrategy", "", "Daemon/Replica/Sidecar")))
	if !ok {
		panic("Deployment Strategy not provided")
	}
	e.EcsDeploymentStrategy = deploymentStrategy

	e.EcsClusterArn = *(flags.String("clusterArn", "", "Used to restart ecs task to apply new agent config"))
	e.CwagentConfigSsmParamName = *(flags.String("cwagentConfigSsmParamName", "", "Used to set new cwa config"))
	e.EcsServiceName = *(flags.String("cwagentECSServiceName", "", "Used to restart ecs task to apply new agent config"))

	return e
}

func GetEnvironmentMetaData(flagArgs string) *MetaData {
	flags := flag.NewFlagSet(flagArgs, flag.ContinueOnError)
	metaData := &(MetaData{})
	metaData = fillComputeType(flags, metaData)
	metaData = fillECSData(flags, metaData)

	return metaData
}
