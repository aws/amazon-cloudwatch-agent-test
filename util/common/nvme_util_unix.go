// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package common

import (
	"fmt"
	"os"
	"strings"
)

const sysClassNvmeDirPath = "/sys/class/nvme"

// GetAnyEBSVolumeID will return the volume ID of the first NVMe device found
func GetAnyEBSVolumeID() (string, error) {
	entries, err := os.ReadDir(sysClassNvmeDirPath)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		modelPath := fmt.Sprintf("%s/%s/model", sysClassNvmeDirPath, entry.Name())
		modelData, err := os.ReadFile(modelPath)
		if err != nil {
			continue // skip if can't read model
		}
		model := strings.TrimSpace(string(modelData))
		if model != "Amazon Elastic Block Store" {
			continue // skip if not the EBS model
		}

		serialPath := fmt.Sprintf("%s/%s/serial", sysClassNvmeDirPath, entry.Name())
		serialData, err := os.ReadFile(serialPath)
		if err != nil {
			continue // skip if can't read serial
		}
		trimmed := strings.TrimPrefix(strings.TrimSpace(string(serialData)), "vol")
		if trimmed != "" {
			return "vol-" + trimmed, nil
		}
	}

	return "", fmt.Errorf("could not find an EBS NVMe device")
}

// GetAnyInstanceStoreSerialID returns the serial ID of the first NVMe instance store device found.
func GetAnyInstanceStoreSerialID() (string, error) {
	entries, err := os.ReadDir(sysClassNvmeDirPath)
	if err != nil {
		return "", fmt.Errorf("failed to read NVMe devices: %v", err)
	}

	for _, entry := range entries {
		modelPath := fmt.Sprintf("%s/%s/model", sysClassNvmeDirPath, entry.Name())
		modelData, err := os.ReadFile(modelPath)
		if err != nil {
			continue // skip if can't read model
		}
		model := strings.TrimSpace(string(modelData))
		if model != "Amazon EC2 NVMe Instance Storage" {
			continue // skip if not the instance storage model
		}

		serialPath := fmt.Sprintf("%s/%s/serial", sysClassNvmeDirPath, entry.Name())
		serialData, err := os.ReadFile(serialPath)
		if err != nil {
			continue // skip if can't read serial
		}
		serial := strings.TrimSpace(string(serialData))
		if serial != "" {
			return serial, nil
		}
	}

	return "", fmt.Errorf("could not find any NVMe instance store device")
}
