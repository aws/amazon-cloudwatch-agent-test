// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package soak

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
)

const (
	configInputPath = "resources/soak_config.json"
)

// TestStartSoak just does setup.
// It starts the agent and starts some background processes which generate
// load and monitor for resource leaks.
// The agent config should use a mocked backend (local stack)to save cost.
func TestStartSoak(t *testing.T) {
	require.NoError(t, startLocalStack())
	common.CopyFile(configInputPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false))
	require.NoError(t, startLoadGen(10, 4000, 120))
	require.NoError(t, startValidator(50, 170000000))
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
func startLoadGen(fileNum int, linesPerSecond int, lineSizeBytes int) error {
	err := killExisting("log-generator")
	if err != nil {
		return err
	}
	// Assume PWD is the .../test/soak/ directory.
	cmd := fmt.Sprintf("go run ../../cmd/log-generator -fileNum=%d -eventRatio=%d -eventSize=%d -path /tmp/soakTest",
		fileNum, linesPerSecond, lineSizeBytes)
	return common.RunAyncCommand(cmd)
}

func startValidator(cpuLimit int, memLimit int) error {
	err := killExisting("log-generator")
	if err != nil {
		return err
	}
	// Assume PWD is the .../test/soak/ directory.
	cmd := fmt.Sprintf("go run ../../cmd/validator -cpuLimit=%d, -memLimit=%d",
		cpuLimit, memLimit)
	return common.RunAyncCommand(cmd)
}

// killExisting will search the command line of every process and kill any
// tyhat match
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
