// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ssm_document

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// ShellInfo contains information about the detected shell
type ShellInfo struct {
	ShellPath string
	ShellType string
	IsPOSIX   bool
}

// GetShellType returns the shell type for /bin/sh
func GetShellType() (*ShellInfo, error) {
	// Use readlink to resolve the /bin/sh symlink
	cmd := exec.Command("readlink", "-f", "/bin/sh")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve /bin/sh symlink: %w", err)
	}

	shellPath := strings.TrimSpace(string(output))
	shellType := "unknown"
	isPOSIX := false

	// Determine shell type based on the resolved path
	if strings.Contains(shellPath, "dash") {
		shellType = "dash"
		isPOSIX = true
	} else if strings.Contains(shellPath, "bash") {
		shellType = "bash"
		isPOSIX = true
	} else if strings.Contains(shellPath, "sh") {
		// Generic sh, assume POSIX-compliant
		shellType = "sh"
		isPOSIX = true
	}

	return &ShellInfo{
		ShellPath: shellPath,
		ShellType: shellType,
		IsPOSIX:   isPOSIX,
	}, nil
}

// VerifyShellCompatibility checks if the system shell is compatible and logs the information
func VerifyShellCompatibility() error {
	shellInfo, err := GetShellType()
	if err != nil {
		return fmt.Errorf("shell compatibility check failed: %w", err)
	}

	log.Printf("Shell compatibility check:")
	log.Printf("  /bin/sh resolves to: %s", shellInfo.ShellPath)
	log.Printf("  Detected shell type: %s", shellInfo.ShellType)
	log.Printf("  POSIX-compliant: %v", shellInfo.IsPOSIX)

	if !shellInfo.IsPOSIX {
		log.Printf("WARNING: Shell may not be POSIX-compliant")
	}

	return nil
}
