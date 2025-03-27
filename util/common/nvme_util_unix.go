// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package common

import (
	"fmt"
	"os"
	"strings"
)

const SysClassNvmeDirPath = "/sys/class/nvme"

// GetAnyNvmeVolumeID will return the volume ID of the first NVMe device found
func GetAnyNvmeVolumeID() (string, error) {
	entries, err := os.ReadDir(SysClassNvmeDirPath)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		data, err := os.ReadFile(fmt.Sprintf("%s/%s/serial", SysClassNvmeDirPath, entry.Name()))
		if err != nil {
			return "", nil
		}
		trimmed := strings.TrimPrefix(cleanupString(string(data)), "vol")
		// Just take the first entry
		return "vol-" + trimmed, nil
	}

	return "", fmt.Errorf("could not find an nvme device")
}

func cleanupString(input string) string {
	// Some device info strings use fixed-width padding and/or end with a new line
	return strings.TrimSpace(strings.TrimSuffix(input, "\n"))
}
