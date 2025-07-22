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

// PatchType represents the type of patch operation
type PatchType string

const (
	PatchTypeMerge PatchType = "merge"
	PatchTypeJSON  PatchType = "json"
)

// K8CtlManager manages Kubernetes operations using kubectl or oc commands
type K8CtlManager struct {
	Command string // Command to use (kubectl or oc)
}

// NewK8CtlManager creates a new K8CtlManager instance with kubectl as default
func NewK8CtlManager(env *environment.MetaData) *K8CtlManager {
	return &K8CtlManager{Command: "kubectl"}
}

// ApplyResource creates or updates resources from a Kubernetes YAML manifest
func (k *K8CtlManager) ApplyResource(manifestPath string) error {
	cmd := exec.Command(k.Command, "apply", "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// DeleteResource removes resources defined in a Kubernetes YAML manifest
func (k *K8CtlManager) DeleteResource(manifestPath string) error {
	cmd := exec.Command(k.Command, "delete", "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// DeleteSpecificResource removes a specific Kubernetes resource by type and name
func (k *K8CtlManager) DeleteSpecificResource(resourceType, resourceName, namespace string) error {
	cmd := exec.Command(k.Command, "delete", resourceType, resourceName, "-n", namespace, "--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("failed to delete resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// UpdateKubeConfig updates the kubeconfig file for an EKS cluster
func (k *K8CtlManager) UpdateKubeConfig(clusterName string) error {
	cmd := exec.Command("aws", "eks", "update-kubeconfig", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}
	return nil
}

// ExecuteCommandOnPod runs a command in a specified pod
func (k *K8CtlManager) ExecuteCommandOnPod(podName, namespace string, command []string) (string, error) {
	cmdArgs := append([]string{"exec", podName, "-n", namespace, "--"}, command...)
	cmd := exec.Command(k.Command, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command on pod %s: %w\nOutput: %s", podName, err, output)
	}
	return string(output), nil
}

// Execute runs a kubectl/oc command with the given arguments
func (k *K8CtlManager) Execute(cmdArgs []string) (string, error) {
	cmd := exec.Command(k.Command, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command %s:\nOutput: %s", err, output)
	}
	return string(output), nil
}

// ConditionalWait blocks until a condition is met for a Kubernetes resource
func (k *K8CtlManager) ConditionalWait(condition string, timeout time.Duration, resource string, namespace string) error {
	startTime := time.Now()
	for {
		wait := exec.Command(k.Command, "wait", condition, fmt.Sprintf("--timeout=%s", timeout.String()), resource, "-n", namespace)
		output, err := wait.CombinedOutput()
		if err == nil {
			return nil
		}
		if time.Since(startTime) > timeout {
			return fmt.Errorf("failed to wait for %s: %w\nOutput: %s", resource, err, output)
		}
		time.Sleep(5 * time.Second) // Wait before retrying
	}
}

// PatchResource applies a patch to a Kubernetes resource
func (k *K8CtlManager) PatchResource(resourceType, resourceName, namespace string, patchType PatchType, patchData string) error {
	cmd := exec.Command(k.Command, "patch", resourceType, resourceName,
		"-n", namespace,
		"--type", string(patchType),
		"-p", patchData)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to patch resource %s/%s: %w\nOutput: %s", resourceType, resourceName, err, output)
	}
	return nil
}
