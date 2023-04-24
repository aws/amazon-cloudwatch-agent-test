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

type testConfig struct {
	logFileCount int
	linesPerSecond int
	lineSizeBytes int
	emfFileCount int
	eventsPerSecond int
	statsdClientCount int
	tps int
	metricCount int
}

func TestSoakLow(t *testing.T) {
	tc := testConfig{
		logFileCount: 10,
		linesPerSecond: 10,
		lineSizeBytes: 100,
		emfFileCount: 10,
		eventsPerSecond: 10,
		statsdClientCount: 5,
		tps: 100,
		metricCount: 100,
	}
	switch runtime.GOOS {
	case "darwin":
		runTest(t, "SoakTestLowDarwin", "resources/soak_darwin.json", tc)
	case "linux":
		runTest(t, "SoakTestLowLinux", "resources/soak_linux.json", tc)
	case "windows":
		runTest(t, "SoakTestLowWindows", "resources/soak_windows.json",tc)
	}
}

func TestSoakMedium(t *testing.T) {
	tc := testConfig{
		logFileCount: 10,
		linesPerSecond: 100,
		lineSizeBytes: 100,
		emfFileCount: 10,
		eventsPerSecond: 100,
		statsdClientCount: 5,
		tps: 100,
		metricCount: 100,
	}
	switch runtime.GOOS {
	case "darwin":
		runTest(t, "SoakTestMediumDarwin", "resources/soak_darwin.json", tc)
	case "linux":
		runTest(t, "SoakTestMediumLinux", "resources/soak_linux.json", tc)
	case "windows":
		runTest(t, "SoakTestMediumWindows", "resources/soak_windows.json", tc)
	}
}

func TestSoakHigh(t *testing.T) {
	tc := testConfig{
		logFileCount: 10,
		linesPerSecond: 1000,
		lineSizeBytes: 100,
		emfFileCount: 10,
		eventsPerSecond: 1000,
		statsdClientCount: 5,
		tps: 100,
		metricCount: 100,
	}
	switch runtime.GOOS {
	case "darwin":
		runTest(t, "SoakTestHighDarwin", "resources/soak_darwin.json", tc)
	case "linux":
		runTest(t, "SoakTestHighLinux", "resources/soak_linux.json", tc)
	case "windows":
		runTest(t, "SoakTestHighWindows", "resources/soak_windows.json", tc)
	}
}

// runTest just does setup.
// It starts the agent and starts some background processes which generate
// load.
// The agent config should use a mocked backend (local stack)to save cost.
func runTest(t *testing.T, testName string, configPath string, tc testConfig) {
	require.NoError(t, startLocalStack())
	common.CopyFile(configPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false))
	require.NoError(t, startLogGen(tc.logFileCount, tc.linesPerSecond, tc.lineSizeBytes))
	require.NoError(t, startEMFGen(tc.emfFileCount, tc.eventsPerSecond))
	require.NoError(t, startStatsd(tc.statsdClientCount, tc.tps, tc.metricCount))
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
