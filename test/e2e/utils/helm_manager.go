package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HelmValueType is a string-based enum for Helm value types
type HelmValueType string

// Enum values for HelmValueType
const (
	HelmValueText HelmValueType = "text" // Default
	HelmValueJSON HelmValueType = "json"
)

// IsValid checks if the HelmValueType is a valid enum value
func (hvt HelmValueType) IsValid() bool {
	switch hvt {
	case HelmValueText, HelmValueJSON:
		return true
	default:
		return false
	}
}

// HelmManager handles Helm operations.
type HelmManager struct{}

// NewHelmManager creates a new instance of HelmManager.
func NewHelmManager() *HelmManager {
	return &HelmManager{}
}

// HelmValue represents a Helm chart value with a type
type HelmValue struct {
	Value string        `json:"value"`
	Type  HelmValueType `json:"type,omitempty"`
}

// NewHelmValue creates a new HelmValue with the given value and default text type
func NewHelmValue(value string) HelmValue {
	return HelmValue{
		Value: value,
		Type:  HelmValueText,
	}
}

// InstallOrUpdate installs or upgrades a Helm release.
func (h *HelmManager) InstallOrUpdate(releaseName, chartPath string, values map[string]HelmValue, namespace string) error {
	args := []string{"upgrade", "--install", releaseName, chartPath, "--namespace", namespace, "--create-namespace"}

	// Convert values map to --set flags
	for key, value := range values {
		if !value.Type.IsValid() {
			return fmt.Errorf("invalid helm value type: %s for %s", value.Type, key)
		}
		if value.Value == "" {
			continue
		}
		setFlag := "--set"
		if value.Type == HelmValueJSON {
			setFlag = "--set-json"
		}
		args = append(args, setFlag, fmt.Sprintf("%s=%v", key, value.Value))
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
