// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	sleepForFlush    = 20 * time.Second // default flush interval is 5 seconds
)

// Validate tests journald log collection functionality
func Validate() error {
	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)

	configPath := "resources/config.json"
	logGroupName := "journald-test-" + instanceId
	logStreamName := instanceId
	testUnits := []string{"systemd", "sshd"} // Units to test
	expectedMinLogs := 3 // Minimum expected logs

	// Clean up any existing log groups
	defer awsservice.DeleteLogGroupAndStream(logGroupName, logStreamName)

	return runJournaldTest(configPath, logGroupName, logStreamName, testUnits, expectedMinLogs)
}

func runJournaldTest(configPath, logGroupName, logStreamName string, testUnits []string, expectedMinLogs int) error {
	// Clean up agent logs
	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)

	start := time.Now()

	// Copy configuration and start agent
	common.CopyFile(configPath, configOutputPath)
	if err := common.StartAgent(configOutputPath, true, false); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Wait for agent to initialize
	time.Sleep(sleepForFlush)

	// Generate test log entries for specified units
	if err := generateTestLogs(testUnits, expectedMinLogs); err != nil {
		log.Printf("Warning: failed to generate test logs: %v", err)
		// Continue with test as there might be existing logs
	}

	// Wait for logs to be processed and sent
	time.Sleep(sleepForFlush)

	// Stop agent
	common.StopAgent()
	end := time.Now()

	// Validate logs were sent to CloudWatch
	return awsservice.ValidateLogs(
		logGroupName,
		logStreamName,
		&start,
		&end,
		func(events []types.OutputLogEvent) error {
			if len(events) < expectedMinLogs {
				return fmt.Errorf("expected at least %d logs, got %d", expectedMinLogs, len(events))
			}
			log.Printf("Successfully validated %d journald log events", len(events))
			return nil
		},
	)
}

// generateTestLogs creates test log entries in journald for the specified units
func generateTestLogs(units []string, count int) error {
	for _, unit := range units {
		for i := 0; i < count; i++ {
			message := fmt.Sprintf("Test journald log entry %d for unit %s - timestamp %s",
				i+1, unit, time.Now().Format(time.RFC3339))

			// Use logger command to write to journald with specific unit identifier
			cmd := exec.Command("logger", "-t", unit, message)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to write log for unit %s: %w", unit, err)
			}

			// Small delay between log entries
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}