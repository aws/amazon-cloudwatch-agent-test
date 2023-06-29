// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package soak

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type testConfig struct {
	logFileCount      int
	linesPerSecond    int
	lineSizeBytes     int
	emfFileCount      int
	eventsPerSecond   int
	statsdClientCount int
	tps               int
	metricCount       int
}

func TestSoakHigh(t *testing.T) {
	tc := testConfig{
		logFileCount:      10,
		linesPerSecond:    4000,
		lineSizeBytes:     120,
		emfFileCount:      2,
		eventsPerSecond:   1600,
		statsdClientCount: 5,
		tps:               100,
		metricCount:       100,
	}
	switch runtime.GOOS {
	case "darwin":
		runTest(t, "resources/soak_high_darwin.json", tc)
	case "linux":
		runTest(t, "resources/soak_high_linux.json", tc)
	case "windows":
		runTest(t, "resources/soak_high_windows.json", tc)
	}
}

func runTest(t *testing.T, configPath string, tc testConfig) {
	common.CopyFile(configPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false, false))
	require.NoError(t, startLogGen(tc.logFileCount, tc.linesPerSecond, tc.lineSizeBytes))
	require.NoError(t, startEMFGen(tc.emfFileCount, tc.eventsPerSecond))
	require.NoError(t, startStatsd(tc.statsdClientCount, tc.tps, tc.metricCount))
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
	return common.RunAsyncCommand(cmd)
}

// startEMFGen starts a long running process that writes EMF events to log files.
func startEMFGen(fileNum int, eventsPerSecond int) error {
	err := killExisting("emf-generator")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("go run ../../cmd/emf-generator -fileNum=%d -eventsPerSecond=%d",
		fileNum, eventsPerSecond)
	return common.RunAsyncCommand(cmd)
}

// startStatsd starts a long running process that sends statsd metrics to a port.
func startStatsd(clientNum int, tps int, metricNum int) error {
	err := killExisting("statsd-generator")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("go run ../../cmd/statsd-generator -clientNum=%d -tps=%d -metricNum=%d",
		clientNum, tps, metricNum)
	return common.RunAsyncCommand(cmd)
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
