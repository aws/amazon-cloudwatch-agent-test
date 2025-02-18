package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HelmManager handles Helm operations.
type HelmManager struct{}

// NewHelmManager creates a new instance of HelmManager.
func NewHelmManager() *HelmManager {
	return &HelmManager{}
}

// InstallOrUpdate installs or upgrades a Helm release.
func (h *HelmManager) InstallOrUpdate(releaseName, chartPath string, values map[string]any, namespace string) error {
	args := []string{"upgrade", "--install", releaseName, chartPath, "--namespace", namespace, "--create-namespace"}

	// Convert values map to --set flags
	for key, value := range values {
		if value.(string) == "" {
			continue
		}
		args = append(args, "--set", fmt.Sprintf("%s=%v", key, value))
	}

	helmCmd := exec.Command("helm", args...)
	helmCmd.Stdout = os.Stdout
	helmCmd.Stderr = os.Stderr
	if err := helmCmd.Run(); err != nil {
		return fmt.Errorf("failed to install/update Helm release: %w \n %s", err,
			strings.Join(helmCmd.Args, " "))
	}

	return nil
}

// Uninstall removes a Helm release.
func (h *HelmManager) Uninstall(releaseName, namespace string) error {
	helmCmd := exec.Command("helm", "uninstall", releaseName, "--namespace", namespace)
	if err := helmCmd.Run(); err != nil {
		return fmt.Errorf("failed to uninstall Helm release: %w", err)
	}
	return nil
}
