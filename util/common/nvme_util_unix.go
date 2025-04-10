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
