package dimension

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
)

type EFMECSDimensionProvider struct {
	Provider
}

func (p EFMECSDimensionProvider) IsApplicable() bool {
	return p.env.ComputeType == computetype.ECS
}

func (p EFMECSDimensionProvider) GetDimension(instruction Instruction) types.Dimension {
	//if instruction.Key == "ClusterName" {
	//	return types.Dimension{
	//		Name:  aws.String("ClusterName"),
	//		Value: aws.String( /*p.Provider.env.EcsClusterName +*/ "cluster"),
	//	}
	//}
	//
	//if instruction.Key == "ContainerInstanceId" {
	//	//TODO currently assuming there's only one container
	//	//containerInstances, err := awsservice.GetContainerInstances(p.Provider.env.EcsClusterArn)
	//	//if err != nil {
	//	//	log.Print(err)
	//	//	return types.Dimension{}
	//	//}
	//
	//	return types.Dimension{
	//		Name:  aws.String("ContainerInstanceId"),
	//		Value: aws.String( /*containerInstances[0].ContainerInstanceId +*/ "containerID"),
	//	}
	//}

	if instruction.Key == "InstanceId" {
		//TODO currently assuming there's only one container
		//containerInstances, err := awsservice.GetContainerInstances(p.Provider.env.EcsClusterArn)
		//if err != nil {
		//	log.Print(err)
		//	return types.Dimension{}
		//}

		return types.Dimension{
			Name:  aws.String("InstanceId"),
			Value: aws.String( /*containerInstances[0].ContainerInstanceArn +*/ "INSTANCEID"),
		}
	}

	return types.Dimension{}
}

func (p EFMECSDimensionProvider) Name() string {
	return "EMFECSProvider"
}

var _ IProvider = (*EFMECSDimensionProvider)(nil)
