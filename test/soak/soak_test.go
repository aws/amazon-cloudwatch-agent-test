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

func TestSoakLow(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		// todo:
	case "linux":
		runTest(t, "SoakTestLowLinux", "resources/soak_linux.json", 5, 100_000_000)
	case "windows":
		runTest(t, "SoakTestLowLinux", "resources/soak_windows.json", 5, 100_000_000)
	}
}

// todo: add high througput
// todo: logrotate
// todo: multiple-logs
func TestSoakHigh(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		// todo:
	case "linux":
		runTest(t, "SoakTestHighLinux", "resources/soak_linux.json", 50, 200_000_000)
	case "windows":
		runTest(t, "SoakTestHighWindows", "resources/soak_windows.json", 50, 200_000_000)
	}
}

// runTest just does setup.
// It starts the agent and starts some background processes which generate
// load and monitor for resource leaks.
// The agent config should use a mocked backend (local stack)to save cost.
// testName is used as the namespace for validator metrics.
func runTest(t *testing.T, testName string, configPath string, cpuLimit int, memLimit int) {
	require.NoError(t, startLocalStack())
	common.CopyFile(configPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false))
	require.NoError(t, startValidator(testName, cpuLimit, memLimit))
	if strings.Contains(testName, "Low") {
		require.NoError(t, startLogGen(10, 10, 100))
		require.NoError(t, startEMFGen(10, 10))
		require.NoError(t, startStatsd(1, 100, 10))
	} else if strings.Contains(testName, "Medium") {
		require.NoError(t, startLogGen(10, 100, 100))
		require.NoError(t, startEMFGen(10, 100))
		require.NoError(t, startStatsd(2, 100, 100))
	} else if strings.Contains(testName, "High") {
		require.NoError(t, startLogGen(10, 1000, 100))
		require.NoError(t, startEMFGen(10, 1000))
		require.NoError(t, startStatsd(10, 100, 1000))
	} else {
		require.Fail(t, "unexpected test name, %s", testName)
	}
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

// startLogGen starts a long running process that writes lines to log files.
func startLogGen(fileNum int, eventsPerSecond int, eventSize int) error {
	err := killExisting("log-generator")
	if err != nil {
		return err
	}

	// Assume PWD is the .../test/soak/ directory.
	cmd := fmt.Sprintf("go run ../../cmd/log-generator -fileNum=%d -eventsPerSecond=%d -eventSize=%d",
		fileNum, eventsPerSecond, eventSize)
	return common.RunAyncCommand(cmd)
}

// startEMFGen starts a long running process that writes EMF events to log files.
func startEMFGen(fileNum int, eventsPerSecond int) error {
	err := killExisting("emf-generator")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("go run ../../cmd/emf-generator -fileNum=%d -eventsPerSecond=%d",
		fileNum, eventsPerSecond)
	return common.RunAyncCommand(cmd)
}

// startStatsd starts a long running process that sends statsd metrics to a port.
func startStatsd(clientNum int, tps int, metricNum int) error {
	err := killExisting("statsd-generator")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("go run ../../cmd/statsd-generator -clientNum=%d -tps=%d -metricNum=%d",
		clientNum, tps, metricNum)
	return common.RunAyncCommand(cmd)
}


func startValidator(testName string, cpuLimit int, memLimit int) error {
	err := killExisting("soak-validator")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("go run ../../cmd/soak-validator -testName=%s -cpuLimit=%d -memLimit=%d",
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
			err = p.Kill()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
