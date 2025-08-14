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

// GetAnyNvmeVolumeID will return the volume ID of the first NVMe device found
func GetAnyNvmeVolumeID() (string, error) {
	entries, err := os.ReadDir(sysClassNvmeDirPath)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		data, err := os.ReadFile(fmt.Sprintf("%s/%s/serial", sysClassNvmeDirPath, entry.Name()))
		if err != nil {
			return "", nil
		}
		trimmed := strings.TrimPrefix(strings.TrimSpace(string(data)), "vol")
		// Just take the first entry
		return "vol-" + trimmed, nil
	}

	return "", fmt.Errorf("could not find an nvme device")
}

// GetAnyInstanceStoreSerialID returns the serial ID of the first NVMe instance store device found.
func GetAnyInstanceStoreSerialID() (string, error) {
	entries, err := os.ReadDir(sysClassNvmeDirPath)
	if err != nil {
		return "", fmt.Errorf("failed to read NVMe devices: %v", err)
	}

	for _, entry := range entries {
		data, err := os.ReadFile(fmt.Sprintf("%s/%s/serial", sysClassNvmeDirPath, entry.Name()))
		if err != nil {
			continue // skip devices we can't read
		}
		serial := strings.TrimSpace(string(data))
		if serial != "" {
			return serial, nil
		}
	}

	return "", fmt.Errorf("could not find any NVMe instance store device")
}
