// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//------------------------------------------------------------------------------
// Environment Setup
//------------------------------------------------------------------------------

func InitializeEnvironment(env *environment.MetaData) error {
	if env.Region != "us-west-2" {
		if err := awsservice.ConfigureAWSClients(env.Region); err != nil {
			return fmt.Errorf("failed to reconfigure AWS clients: %v", err)
		}
		fmt.Printf("AWS clients reconfigured to use region: %s\n", env.Region)
	} else {
		fmt.Printf("Using default testing region: us-west-2\n")
	}

	fmt.Println("Applying K8s resources...")
	if err := ApplyResources(env); err != nil {
		return fmt.Errorf("failed to apply K8s resources: %v", err)
	}

	return nil
}

//------------------------------------------------------------------------------
// K8s Resource Management Functions
//------------------------------------------------------------------------------

func ApplyResources(env *environment.MetaData) error {
	updateKubeconfig := exec.Command("aws", "eks", "update-kubeconfig", "--name", env.EKSClusterName)
	output, err := updateKubeconfig.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}

	fmt.Println("Installing Helm release...")
	helm := []string{
		"helm", "upgrade", "--install", "amazon-cloudwatch-observability",
		filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability"),
		"--set", fmt.Sprintf("clusterName=%s", env.EKSClusterName),
		"--set", fmt.Sprintf("region=%s", env.Region),
		"--set", fmt.Sprintf("agent.image.repository=%s", env.CloudwatchAgentRepository),
		"--set", fmt.Sprintf("agent.image.tag=%s", env.CloudwatchAgentTag),
		"--set", fmt.Sprintf("agent.image.repositoryDomainMap.public=%s", env.CloudwatchAgentRepositoryURL),
		"--set", fmt.Sprintf("manager.image.repository=%s", env.CloudwatchAgentOperatorRepository),
		"--set", fmt.Sprintf("manager.image.tag=%s", env.CloudwatchAgentOperatorTag),
		"--set", fmt.Sprintf("manager.image.repositoryDomainMap.public=%s", env.CloudwatchAgentOperatorRepositoryURL),
		"--namespace", "amazon-cloudwatch",
		"--create-namespace",
	}

	if env.AgentConfig != "" {
		agentConfigContent, err := os.ReadFile(env.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
		helm = append(helm, "--set-json", fmt.Sprintf("agent.config=%s", string(agentConfigContent)))
	}

	helmUpgrade := exec.Command(helm[0], helm[1:]...)
	helmUpgrade.Stdout = os.Stdout
	helmUpgrade.Stderr = os.Stderr
	if err := helmUpgrade.Run(); err != nil {
		return fmt.Errorf("failed to install Helm release: %w", err)
	}

	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	wait := exec.Command("kubectl", "wait", "--for=condition=available", "--timeout=60s", "deployment/amazon-cloudwatch-observability-controller-manager", "-n", "amazon-cloudwatch")
	output, err = wait.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for operator deployment: %w\nOutput: %s", err, output)
	}

	deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yaml")

	apply := exec.Command("kubectl", "apply", "-f", env.SampleApp)
	output, err = apply.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply sample app: %w\nOutput: %s", err, output)
	}

	fmt.Println("Waiting for Sample Application to initialize...")
	wait = exec.Command("kubectl", "wait", "--for=condition=available", "--timeout=300s", fmt.Sprintf("deployment/%s", deploymentName), "-n", "test")
	output, err = wait.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for deployment %s: %w\nOutput: %s", deploymentName, err, output)
	}

	return nil
}

func DestroyResources(env *environment.MetaData) error {
	updateKubeconfig := exec.Command("aws", "eks", "update-kubeconfig", "--name", env.EKSClusterName)
	output, err := updateKubeconfig.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}

	var errors []error

	fmt.Println("Deleting test namespace...")
	deleteCmd := exec.Command("kubectl", "delete", "namespace", "test", "--timeout=60s")
	output, err = deleteCmd.CombinedOutput()

	// We don't want to consider not finding the namespace to be an error since that's the outcome we want
	if err != nil && !strings.Contains(string(output), "not found") {
		errors = append(errors, fmt.Errorf("failed to delete test namespace: %w\nOutput: %s", err, output))
	}

	fmt.Println("Uninstalling Helm release...")
	helm := []string{
		"helm", "uninstall", "amazon-cloudwatch-observability", "--namespace", "amazon-cloudwatch",
	}

	helmUninstall := exec.Command(helm[0], helm[1:]...)
	helmUninstall.Stdout = os.Stdout
	helmUninstall.Stderr = os.Stderr
	if err := helmUninstall.Run(); err != nil {
		errors = append(errors, fmt.Errorf("failed to uninstall Helm release: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %v", errors)
	}

	return nil
}
