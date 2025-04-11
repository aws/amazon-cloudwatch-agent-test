// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
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
		fmt.Printf("Using default testing region: us-west-2\n")
	}

	fmt.Println("Applying K8s resources...")
	if err := ApplyResources(k8ctl, env); err != nil {
		return fmt.Errorf("failed to apply K8s resources: %v", err)
	}

	return nil
}

func ApplyResources(k8ctl *utils.K8CtlManager, env *environment.MetaData) error {
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	if err := updateImages(k8ctl, env); err != nil {
		return fmt.Errorf("failed to update images: %v", err)
	}

	if env.AgentConfig != "" {
		if _, err := os.ReadFile(env.AgentConfig); err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
	}

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
	if err := k8ctl.ConditionalWait("--for=condition=available", 300*time.Second,
		fmt.Sprintf("deployment/%s", deploymentName), "test"); err != nil {
		return err
	}

	return nil
}

func DestroyResources(env *environment.MetaData) error {
	k8ctl := utils.NewK8CtlManager(env)
	if err := k8ctl.UpdateKubeConfig(env.EKSClusterName); err != nil {
		return err
	}

	return k8ctl.DeleteSpecificResource("namespace", "test", "default")
}

func updateImages(k8ctl *utils.K8CtlManager, env *environment.MetaData) error {
	// Patch CloudWatch agent
	if env.CloudwatchAgentTag != "latest" {
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
	}

	// Patch operator
	if env.CloudwatchAgentOperatorTag != "latest" {
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
	}

	return nil
}
