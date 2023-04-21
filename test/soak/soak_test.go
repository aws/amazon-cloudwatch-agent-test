// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package soak

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
)

func TestSoak(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		// todo:
	case "linux":
		runTest(t, "SoakTestLinux", "resources/soak_linux.json")
	case "windows":
		runTest(t, "SoakTestLinux", "resources/soak_windows.json")
	}
}

// todo: add high througput
// todo: logrotate
// todo: multiple-logs
func TestSoakHighLoad(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		// todo:
	case "linux":
		runTest(t, "SoakTestHighLoadLinux", "resources/soak_linux.json")
	case "windows":
		runTest(t, "SoakTestHighLoadWindows", "resources/soak_windows.json")
	}
}


// runTest just does setup.
// It starts the agent and starts some background processes which generate
// load and monitor for resource leaks.
// The agent config should use a mocked backend (local stack)to save cost.
// testName is used as the namespace for validator metrics.
func runTest(t *testing.T, testName string, configPath string) {
	require.NoError(t, startLocalStack())
	common.CopyFile(configPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false))
	if strings.Contains(testName, "HighLoad") {
		require.NoError(t, startLoadGen(10, 4000, 120))
	} else {
		require.NoError(t, startLoadGen(10, 100, 120))
	}
	require.NoError(t, startValidator(testName, 50, 170000000))
}

// Refer to https://github.com/localstack/localstack
func startLocalStack() error {
	// Kill existing. (Assumes 0 or 1 container ids found)
	cmd := "docker ps --filter ancestor=localstack/localstack --quiet"
	id, _ := common.RunCommand(cmd)
	if id != "" {
		cmd = "docker kill " + id
		_, _ = common.RunCommand(cmd)
	}
	cmd = "docker run -d -p 4566:4566 -p 4510-4559:4510-4559 localstack/localstack"
	_, err := common.RunCommand(cmd)
	return err
}

// startLoadGen starts a long running process that writes lines to log files.
func startLoadGen(fileNum int, eventsPerSecond int, eventSize int) error {
	err := killExisting("log-generator")
	if err != nil {
		return err
	}
	// Assume PWD is the .../test/soak/ directory.
	cmd := fmt.Sprintf("go run ../../cmd/log-generator -fileNum=%d -eventsPerSecond=%d -eventSize=%d -path /tmp/soakTest",
		fileNum, eventsPerSecond, eventSize)
	return common.RunAyncCommand(cmd)
}

func startValidator(testName string, cpuLimit int, memLimit int) error {
	err := killExisting("soak-validator")
	if err != nil {
		return err
	}
	// Assume PWD is the .../test/soak/ directory.
	cmd := fmt.Sprintf("go run ../../cmd/soak-validator -testName=%s -cpuLimit=%d, -memLimit=%d",
		testName, cpuLimit, memLimit)
	return common.RunAyncCommand(cmd)
}

// killExisting will search the command line of every process and kill any that match.
// Return nil if no matches found, or if all matches killed successfuly.
func killExisting(name string) error {
	procs, err := process.Processes()
	if err != nil {
		return err
	}
	for _, p := range procs {
		c, _ := p.Cmdline()
		if strings.Contains(c, name) {
			n, _ := p.Name()
			log.Printf("killing process, name %s, cmdline %s", n, c)
			return p.Kill()
		}
	}
	return nil
}
