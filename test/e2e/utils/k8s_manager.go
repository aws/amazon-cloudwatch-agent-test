package utils

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"os/exec"
	"strings"
)

// K8CtlManager handles Kubernetes (kubectl or oc) operations.
type K8CtlManager struct {
	Command string // "kubectl" or "oc"
}

// NewK8CtlManager creates a new instance of K8CtlManager.
func NewK8CtlManager(env *environment.MetaData) *K8CtlManager {
	if env.ComputeType == computetype.ROSA {
		return &K8CtlManager{Command: "oc"}
	}
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

// DeleteResource deletes a Kubernetes resource.
func (k *K8CtlManager) DeleteResource(resourceType, resourceName, namespace string) error {
	cmd := exec.Command(k.Command, "delete", resourceType, resourceName, "-n", namespace, "--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("failed to delete resource: %w\nOutput: %s", err, output)
	}
	return nil
}

// UpdateKubeConfig updates the kubeconfig for an EKS cluster.
func (k *K8CtlManager) UpdateKubeConfig(clusterName string) error {
	cmd := exec.Command("aws", "eks", "update-kubeconfig", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}
	return nil
}
