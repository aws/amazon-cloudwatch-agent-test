// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ssm_document

import (
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

var (
	//go:embed resources/test_amazoncloudwatch_manageagent.json
	manageAgentDoc string
	//go:embed resources/agent_config1.json
	agentConfig1 string
	//go:embed resources/agent_config2.json
	agentConfig2 string
)

// envConfigPath is the on-disk path to the agent's env-config.json on Linux/macOS.
const envConfigPath = "/opt/aws/amazon-cloudwatch-agent/etc/env-config.json"

// platformSetup performs platform-specific initialization (shell compatibility check on unix).
func platformSetup() {
	if err := verifyShellCompatibility(); err != nil {
		log.Printf("Warning: Shell compatibility verification failed: %v", err)
	}
}

// shellInfo contains information about the detected shell.
type shellInfo struct {
	shellPath string
	shellType string
	isPOSIX   bool
}

// getShellType returns the shell type for /bin/sh.
func getShellType() (*shellInfo, error) {
	cmd := exec.Command("readlink", "-f", "/bin/sh")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve /bin/sh symlink: %w", err)
	}

	shellPath := strings.TrimSpace(string(output))
	shellType := "unknown"
	isPOSIX := false

	if strings.Contains(shellPath, "dash") {
		shellType = "dash"
		isPOSIX = true
	} else if strings.Contains(shellPath, "bash") {
		shellType = "bash"
		isPOSIX = true
	} else if strings.Contains(shellPath, "sh") {
		shellType = "sh"
		isPOSIX = true
	}

	return &shellInfo{
		shellPath: shellPath,
		shellType: shellType,
		isPOSIX:   isPOSIX,
	}, nil
}

// verifyShellCompatibility checks if the system shell is compatible and logs the information.
func verifyShellCompatibility() error {
	info, err := getShellType()
	if err != nil {
		return fmt.Errorf("shell compatibility check failed: %w", err)
	}

	log.Printf("Shell compatibility check:")
	log.Printf("  /bin/sh resolves to: %s", info.shellPath)
	log.Printf("  Detected shell type: %s", info.shellType)
	log.Printf("  POSIX-compliant: %v", info.isPOSIX)

	if !info.isPOSIX {
		log.Printf("WARNING: Shell may not be POSIX-compliant")
	}

	return nil
}
