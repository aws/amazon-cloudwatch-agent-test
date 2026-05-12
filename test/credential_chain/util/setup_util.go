// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package util

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	SystemdOverridePath = "/etc/systemd/system/amazon-cloudwatch-agent.service.d/override.conf"
	SystemdOverrideDir  = "/etc/systemd/system/amazon-cloudwatch-agent.service.d"
)

func SetupAgentConfig(templatePath string, namespace string, testName string, user string) error {
	common.CopyFile(templatePath, common.ConfigOutputPath)

	return common.ReplacePlaceholders(common.ConfigOutputPath, map[string]string{
		PlaceholderNamespace: namespace,
		PlaceholderTestName:  testName,
		PlaceholderUser:      user,
	})
}

// SetupSharedCredentialsFile creates a temporary credentials file with specified profile
func SetupSharedCredentialsFile(templatePath string, profile string, accessKey string, secretKey string, sessionToken string, outputPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := common.MkdirAll(dir); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	common.CopyFile(templatePath, outputPath)
	return common.ReplacePlaceholders(outputPath, map[string]string{
		PlaceholderProfile:      profile,
		PlaceholderAccessKey:    accessKey,
		PlaceholderSecretKey:    secretKey,
		PlaceholderSessionToken: sessionToken,
	})
}

// SetupCommonConfig creates common-config.toml with credential configuration
func SetupCommonConfig(templatePath string, profile string, credentialsFile string) error {
	common.CopyFile(templatePath, common.AgentCommonConfigFile)
	return common.ReplacePlaceholders(common.AgentCommonConfigFile, map[string]string{
		PlaceholderProfile:        profile,
		PlaceholderCredentialFile: credentialsFile,
	})
}

// RotateCredentialsFile updates credentials file with new credentials
func RotateCredentialsFile(templatePath, profile, filePath, roleArn, sessionName string, durationSeconds int32) error {
	// Get new credentials via AssumeRole
	creds, err := awsservice.GetCredentials(roleArn, sessionName, durationSeconds)
	if err != nil {
		return fmt.Errorf("failed to get new credentials: %w", err)
	}

	// Backup existing file
	backupPath := filePath + ".bak"
	if _, err = os.Stat(filePath); err == nil {
		common.CopyFile(filePath, backupPath)
	}

	// Write new credentials
	if err = SetupSharedCredentialsFile(templatePath, profile, *creds.AccessKeyId, *creds.SecretAccessKey, *creds.SessionToken, filePath); err != nil {
		// Restore backup on failure
		if _, statErr := os.Stat(backupPath); statErr == nil {
			common.CopyFile(backupPath, filePath)
		}
		return err
	}

	return nil
}

// CleanupCredentialsFile removes temporary credentials file
func CleanupCredentialsFile(filePath string) error {
	if err := common.DeleteFile(filePath); err != nil {
		return fmt.Errorf("failed to remove credentials file %s: %w", filePath, err)
	}
	return nil
}

// ResetCommonConfig empties common-config.toml
func ResetCommonConfig() error {
	if err := common.WriteFile(common.AgentCommonConfigFile, ""); err != nil {
		return fmt.Errorf("failed to reset common-config.toml: %w", err)
	}
	return nil
}

// SetupSystemdOverride creates systemd override with environment variables
func SetupSystemdOverride(overrideContent string) error {
	// Create override directory
	if err := common.MkdirAll(SystemdOverrideDir); err != nil {
		return fmt.Errorf("failed to create systemd override directory: %w", err)
	}

	if err := common.WriteFile(SystemdOverridePath, overrideContent); err != nil {
		return fmt.Errorf("failed to write systemd override content: %w", err)
	}

	return nil
}

// CleanupSystemdOverride removes systemd override configuration
func CleanupSystemdOverride() error {
	if err := common.DeleteFile(SystemdOverridePath); err != nil {
		return fmt.Errorf("failed to remove systemd override: %w", err)
	}
	return nil
}

// ReloadSystemd reloads systemd daemon
func ReloadSystemd() error {
	if output, err := common.RunCommand("sudo systemctl daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w, output: %s", err, output)
	}
	return nil
}
