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

func startAgentAndWaitForInit(t *testing.T) {
	t.Helper()
	common.CopyFile("agent_config.json", configOutputPath)
	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)
	// Wait for journald receiver to initialize
	time.Sleep(30 * time.Second)
}

// waits for logs to be flushed to CloudWatch, before validation.
func waitAndStopAgent() {
	time.Sleep(60 * time.Second)
	common.StopAgent()
}

func TestJournaldUnitsLogs(t *testing.T) {
	startAgentAndWaitForInit(t)

	// Journal entries for cwagent-unit-test.service are created by Terraform provisioner.
	waitAndStopAgent()

	instanceId := awsservice.GetInstanceId()

	err := awsservice.ValidateLogs(
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
	startAgentAndWaitForInit(t)

	instanceId := awsservice.GetInstanceId()

	if err := exec.Command("logger", "-t", "priority-test", "-p", "user.err", "Database connection error occurred").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}
	if err := exec.Command("logger", "-t", "priority-test", "-p", "user.err", "Authentication failed for user").Run(); err != nil {
		t.Logf("warning: logger command failed: %v", err)
	}

	waitAndStopAgent()

	err := awsservice.ValidateLogs(
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
	startAgentAndWaitForInit(t)

	waitAndStopAgent()

	instanceId := awsservice.GetInstanceId()

	err := awsservice.ValidateLogs(
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
	startAgentAndWaitForInit(t)

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

	waitAndStopAgent()

	instanceId := awsservice.GetInstanceId()

	err := awsservice.ValidateLogs(
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
