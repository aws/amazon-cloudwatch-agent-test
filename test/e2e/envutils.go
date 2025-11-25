// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksinstallationtype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func InitializeEnvironment(env *environment.MetaData) error {
	k8ctl := utils.NewK8CtlManager(env)

	if env.ComputeType == computetype.EKS {
		k8ctl.UpdateKubeConfig(env.EKSClusterName)
	}

	if env.Region != "us-west-2" {
		if err := awsservice.ConfigureAWSClients(env.Region); err != nil {
			return fmt.Errorf("failed to reconfigure AWS clients: %v", err)
		}
		fmt.Printf("AWS clients reconfigured to use region: %s\n", env.Region)
	} else {
		fmt.Println("Using default testing region: us-west-2")
	}

	fmt.Println("Applying K8s resources...")
	if env.EKSInstallationType == eksinstallationtype.HELM_CHART {
		helmManager := utils.NewHelmManager()
		if err := applyHelmResources(k8ctl, helmManager, env); err != nil {
			return fmt.Errorf("failed to apply Helm resources: %v", err)
		}
	} else if env.EKSInstallationType == eksinstallationtype.EKS_ADDON {
		if err := applyAddonResources(k8ctl, env); err != nil {
			return fmt.Errorf("failed to apply Addon resources: %v", err)
		}
	} else {
		return fmt.Errorf("invalid EKS installation type: %s. Must be either %s or %s",
			env.EKSInstallationType,
			eksinstallationtype.HELM_CHART,
			eksinstallationtype.EKS_ADDON)
	}

	return nil
}

func applyHelmResources(k8ctl *utils.K8CtlManager, helmManager *utils.HelmManager, env *environment.MetaData) error {
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

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

	// Enable dualstack endpoint for IPv6 tests
	if env.IpFamily == "ipv6" {
		values["useDualstackEndpoint"] = utils.NewHelmValue("true")
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

	if err := helmManager.InstallOrUpdate("amazon-cloudwatch-observability",
		"../../../terraform/eks/e2e/helm-charts/charts/amazon-cloudwatch-observability",
		values, "amazon-cloudwatch"); err != nil {
		return err
	}

	return applyCommonResources(k8ctl, env)
}

func applyAddonResources(k8ctl *utils.K8CtlManager, env *environment.MetaData) error {
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	if err := updateAddonImages(k8ctl, env); err != nil {
		return fmt.Errorf("failed to update images: %v", err)
	}

	if env.AgentConfig != "" {
		if _, err := os.ReadFile(env.AgentConfig); err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
	}

	return applyCommonResources(k8ctl, env)
}

func applyCommonResources(k8ctl *utils.K8CtlManager, env *environment.MetaData) error {
	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	if err := k8ctl.ConditionalWait("--for=condition=available", 4*time.Minute,
		"deployment/amazon-cloudwatch-observability-controller-manager", "amazon-cloudwatch"); err != nil {
		return err
	}

	if err := k8ctl.ApplyResource(env.SampleApp); err != nil {
		return err
	}

	deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yaml")
	fmt.Println("Waiting for Sample Application to initialize...")
	return k8ctl.ConditionalWait("--for=condition=available", 300*time.Second,
		fmt.Sprintf("deployment/%s", deploymentName), "test")
}

func updateAddonImages(k8ctl *utils.K8CtlManager, env *environment.MetaData) error {
	fmt.Printf("Patching CloudWatch Agent with tag: %s\n", env.CloudwatchAgentTag)
	agentPatch := fmt.Sprintf(`{"spec":{"image":"%s/%s:%s"}}`,
		env.CloudwatchAgentRepositoryURL,
		env.CloudwatchAgentRepository,
		env.CloudwatchAgentTag)

	if err := k8ctl.PatchResource(
		"amazoncloudwatchagent",
		"cloudwatch-agent",
		"amazon-cloudwatch",
		utils.PatchTypeMerge,
		agentPatch,
	); err != nil {
		return fmt.Errorf("failed to update CloudWatch agent image: %v", err)
	}

	fmt.Printf("Patching operator with tag: %s\n", env.CloudwatchAgentOperatorTag)
	operatorPatch := fmt.Sprintf(`[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"%s/%s:%s"}]`,
		env.CloudwatchAgentOperatorRepositoryURL,
		env.CloudwatchAgentOperatorRepository,
		env.CloudwatchAgentOperatorTag)

	if err := k8ctl.PatchResource(
		"deployment",
		"amazon-cloudwatch-observability-controller-manager",
		"amazon-cloudwatch",
		utils.PatchTypeJSON,
		operatorPatch,
	); err != nil {
		return fmt.Errorf("failed to update operator image: %v", err)
	}

	fmt.Printf("Patching target allocator with tag: %s\n", env.CloudwatchAgentTargetAllocatorTag)
	targetAllocatorPatch := fmt.Sprintf(`[
				{
					"op": "replace",
					"path": "/spec/template/spec/containers/0/args/6",
					"value": "--target-allocator-image=%s/%s:%s"
				}
			]`,
		env.CloudwatchAgentTargetAllocatorRepositoryURL,
		env.CloudwatchAgentTargetAllocatorRepository,
		env.CloudwatchAgentTargetAllocatorTag)

	if err := k8ctl.PatchResource(
		"deployment",
		"amazon-cloudwatch-observability-controller-manager",
		"amazon-cloudwatch",
		utils.PatchTypeJSON,
		targetAllocatorPatch,
	); err != nil {
		return fmt.Errorf("failed to update target allocator image: %v", err)
	}

	return nil
}

func DestroyResources(env *environment.MetaData) error {
	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	if env.EKSInstallationType == eksinstallationtype.HELM_CHART {
		helmMangager := utils.NewHelmManager()
		if err := helmMangager.Uninstall("amazon-cloudwatch-observability", "amazon-cloudwatch"); err != nil {
			return err
		}
	}

	return k8ctl.DeleteSpecificResource("namespace", "test", "default")
}
