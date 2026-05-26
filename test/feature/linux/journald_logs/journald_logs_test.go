// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_logs

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func systemdVersion() int {
	output, _ := exec.Command("systemctl", "--version").CombinedOutput()
	fields := strings.Fields(strings.Split(string(output), "\n")[0])
	if len(fields) >= 2 {
		var version int
		fmt.Sscanf(fields[1], "%d", &version)
		return version
	}
	return 0
}

func TestJournaldUnitsLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(60 * time.Second)

	// Generate entries under a custom systemd unit
	// Use --wait on systemd 236+ (AL2023) to ensure journal entry is committed
	// Run twice with delay to handle slower journal flush in some regions
	for i := 0; i < 2; i++ {
		args := []string{"systemd-run", "--unit=cwagent-unit-test", "echo", "Unit test message"}
		if systemdVersion() >= 236 {
			args = []string{"systemd-run", "--unit=cwagent-unit-test", "--wait", "echo", "Unit test message"}
		}
		if output, err := exec.Command("sudo", args...).CombinedOutput(); err != nil {
			t.Logf("systemd-run attempt %d failed: %v, output: %s", i+1, err, string(output))
		}
		time.Sleep(30 * time.Second)
	}

	time.Sleep(180 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()

	err = awsservice.ValidateLogs(
		instanceId,
		"journald-units",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message
				if !strings.Contains(message, "cwagent-unit-test.service") {
					return fmt.Errorf("found unexpected log entry %.100s", message)
				}
			}
			log.Printf("All logs validated: %d entries, all matched!", len(events))
			return nil
		},
	)
	assert.NoError(t, err)
}

func TestJournaldPriorityLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(30 * time.Second)

	instanceId := awsservice.GetInstanceId()

	if err := exec.Command("logger", "-t", "priority-test", "-p", "user.err", "Database connection error occurred").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}
	if err := exec.Command("logger", "-t", "priority-test", "-p", "user.err", "Authentication failed for user").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}

	time.Sleep(120 * time.Second)
	common.StopAgent()

	err = awsservice.ValidateLogs(
		instanceId,
		"journald-priority",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message

				if strings.Contains(message, "\"PRIORITY\":\"4\"") ||
					strings.Contains(message, "\"PRIORITY\":\"5\"") ||
					strings.Contains(message, "\"PRIORITY\":\"6\"") ||
					strings.Contains(message, "\"PRIORITY\":\"7\"") {
					return fmt.Errorf("found entry with priority below err: %.100s", message)
				}
			}
			log.Printf("All logs validated: %d entries, all matched!", len(events))
			return nil
		},
	)
	assert.NoError(t, err)
}

func TestJournaldMatchesLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(30 * time.Second)

	time.Sleep(120 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()

	err = awsservice.ValidateLogs(
		instanceId,
		"journald-matches",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message
				if !strings.Contains(message, "\"_UID\":\"0\"") {
					return fmt.Errorf("found entry not matching _UID=0: %.100s", message)
				}
			}
			log.Printf("All logs validated: %d entries, all matched!", len(events))
			return nil
		},
	)
	assert.NoError(t, err)
}

func TestJournaldRegexLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(30 * time.Second)

	// Generate entries that MATCH the include filter: ".*Database.*|.*failed.*"
	if err := exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.err", "Database connection error occurred").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}
	if err := exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.err", "Authentication failed for user").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}
	// Generate entries that should NOT match the filter
	if err := exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.info", "Service started successfully").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}
	if err := exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.info", "Health check passed").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}

	time.Sleep(120 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()

	err = awsservice.ValidateLogs(
		instanceId,
		"journald-regex",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message
				if !strings.Contains(message, "Database") && !strings.Contains(message, "failed") {
					return fmt.Errorf("found entry not matching include regex: %.100s", message)
				}
			}
			log.Printf("All logs validated: %d entries, all matched!", len(events))
			return nil
		},
	)
	assert.NoError(t, err)
}
