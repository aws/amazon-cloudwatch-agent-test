package e2e

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
)

func InitializeEnvironment(env *environment.MetaData) error {
	k8ctl := utils.NewK8CtlManager(env)
	helm := utils.NewHelmManager()

	if env.ComputeType == computetype.EKS {
		k8ctl.UpdateKubeConfig(env.EKSClusterName)
	}

	if env.Region != "us-west-2" {
		// Assume awsservice.ConfigureAWSClients remains unchanged
		if err := awsservice.ConfigureAWSClients(env.Region); err != nil {
			return fmt.Errorf("failed to reconfigure AWS clients: %v", err)
		}
		fmt.Printf("AWS clients reconfigured to use region: %s\n", env.Region)
	} else {
		fmt.Println("Using default testing region: us-west-2")
	}

	fmt.Println("Applying K8s resources...")
	if err := ApplyResources(k8ctl, helm, env); err != nil {
		return fmt.Errorf("failed to apply K8s resources: %v", err)
	}

	return nil
}

func ApplyResources(k8ctl *utils.K8CtlManager, helm *utils.HelmManager, env *environment.MetaData) error {
	// Update kubeconfig
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	// Install Helm chart
	values := map[string]utils.HelmValue{
		"clusterName":                              utils.NewHelmValue(env.EKSClusterName),
		"region":                                   utils.NewHelmValue(env.Region),
		"agent.image.repository":                   utils.NewHelmValue(env.CloudwatchAgentRepository),
		"agent.image.tag":                          utils.NewHelmValue(env.CloudwatchAgentTag),
		"agent.image.repositoryDomainMap.public":   utils.NewHelmValue(env.CloudwatchAgentRepositoryURL),
		"manager.image.repository":                 utils.NewHelmValue(env.CloudwatchAgentOperatorRepository),
		"manager.image.tag":                        utils.NewHelmValue(env.CloudwatchAgentOperatorTag),
		"manager.image.repositoryDomainMap.public": utils.NewHelmValue(env.CloudwatchAgentOperatorRepositoryURL),
	}

	if env.AgentConfig != "" {
		if agentConfigContent, err := os.ReadFile(env.AgentConfig); err == nil {
			values["agent.config"] = utils.HelmValue{
				Value: string(agentConfigContent),
				Type:  utils.HelmValueJSON,
			}
		} else {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
	}

	if err := helm.InstallOrUpdate("amazon-cloudwatch-observability",
		"../../../terraform/eks/e2e/helm-charts/charts/amazon-cloudwatch-observability",
		values, "amazon-cloudwatch"); err != nil {
		return err
	}
	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	err := k8ctl.ConditionalWait("--for=condition=available", 60*time.Second, "deployment/amazon-cloudwatch-observability-controller-manager", "amazon-cloudwatch")
	if err != nil {
		return err
	}
	// Apply sample app
	if env.SampleApp != "" {
		if err := k8ctl.ApplyResource(env.SampleApp); err != nil {
			return err
		}
	}

	return nil
}

func DestroyResources(env *environment.MetaData) error {
	k8ctl := utils.NewK8CtlManager(env)
	helm := utils.NewHelmManager()
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	if env.SampleApp != "" {
		if err := k8ctl.DeleteResource(env.SampleApp); err != nil {
			return err
		}
	}

	// Uninstall Helm release
	if err := helm.Uninstall("amazon-cloudwatch-observability", "amazon-cloudwatch"); err != nil {
		return err
	}

	return nil
}
