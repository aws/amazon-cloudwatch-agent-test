// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package utils

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

// K8CtlManager handles Kubernetes (kubectl or oc) operations.
type K8CtlManager struct {
	Command string // "kubectl" or "oc"
}

// NewK8CtlManager creates a new instance of K8CtlManager.
func NewK8CtlManager(env *environment.MetaData) *K8CtlManager {
	return &K8CtlManager{Command: "kubectl"}
}

// ApplyResource applies a Kubernetes YAML manifest.
func (k *K8CtlManager) ApplyResource(manifestPath string) error {
	cmd := exec.Command(k.Command, "apply", "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply resource: %w\nOutput: %s", err, output)
	}
	return nil
}
func (k *K8CtlManager) DeleteResource(manifestPath string) error {
	cmd := exec.Command(k.Command, "delete", "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// DeleteResource deletes a Kubernetes resource.
func (k *K8CtlManager) DeleteSpecificResource(resourceType, resourceName, namespace string) error {
	cmd := exec.Command(k.Command, "delete", resourceType, resourceName, "-n", namespace, "--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("failed to delete resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// UpdateKubeConfig updates the kubeconfig for an EKS cluster.
func (k *K8CtlManager) UpdateKubeConfig(clusterName string) error {
	if k.Command == "oc" {
		return nil
	}
	cmd := exec.Command("aws", "eks", "update-kubeconfig", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}
	return nil
}

// ExecuteCommandOnPod runs a command inside a specified pod.
func (k *K8CtlManager) ExecuteCommandOnPod(podName, namespace string, command []string) (string, error) {
	cmdArgs := append([]string{"exec", podName, "-n", namespace, "--"}, command...)
	cmd := exec.Command(k.Command, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command on pod %s: %w\nOutput: %s", podName, err, output)
	}
	return string(output), nil
}
func (k *K8CtlManager) Execute(cmdArgs []string) (string, error) {
	cmd := exec.Command(k.Command, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command %s: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}
func (k *K8CtlManager) ConditionalWait(condition string, timeout time.Duration, resource string, namespace string) error {
	wait := exec.Command(k.Command, "wait", condition, fmt.Sprintf("--timeout=%s", timeout.String()), resource, "-n", namespace)
	output, err := wait.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for %s: %w\nOutput: %s", resource, err, output)
	}
	return nil
}
