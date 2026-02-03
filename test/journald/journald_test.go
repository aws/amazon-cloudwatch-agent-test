// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	sleepForFlush    = 20 * time.Second
	testIdentifier   = "cwagent-journald-test"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestJournaldBasicCollection tests basic journald log collection
func TestJournaldBasicCollection(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}
	log.Printf("Found instance id %s", instanceId)

	logGroup := fmt.Sprintf("/aws/cloudwatch-agent/journald/%s", instanceId)
	logStream := instanceId

	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	common.CopyFile("resources/config_journald_basic.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)

	time.Sleep(sleepForFlush)

	// Write test logs to journald using systemd-cat
	testMessage := fmt.Sprintf("Test message from CloudWatch Agent integration test at %s", time.Now().Format(time.RFC3339))
	cmd := fmt.Sprintf("echo '%s' | systemd-cat -t %s -p info", testMessage, testIdentifier)
	common.RunCommand(cmd)

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	// Validate logs appeared in CloudWatch Logs
	err := awsservice.ValidateLogs(
		logGroup,
		logStream,
		&start,
		&end,
		awsservice.AssertLogsCount(1),
	)
	assert.NoError(t, err)
}

// TestJournaldUnitFiltering tests filtering by systemd unit
func TestJournaldUnitFiltering(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}

	logGroup := fmt.Sprintf("/aws/cloudwatch-agent/journald/%s", instanceId)
	logStream := instanceId

	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	common.CopyFile("resources/config_journald_units.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)

	time.Sleep(sleepForFlush)

	// Write logs with matching unit
	matchingMsg := fmt.Sprintf("Matching unit message at %s", time.Now().Format(time.RFC3339))
	common.RunCommand(fmt.Sprintf("echo '%s' | systemd-cat -t cwagent-test -p info", matchingMsg))

	// Write logs with non-matching unit (should be filtered out)
	nonMatchingMsg := fmt.Sprintf("Non-matching unit message at %s", time.Now().Format(time.RFC3339))
	common.RunCommand(fmt.Sprintf("echo '%s' | systemd-cat -t other-unit -p info", nonMatchingMsg))

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	// Should only see 1 log (the matching unit)
	err := awsservice.ValidateLogs(
		logGroup,
		logStream,
		&start,
		&end,
		awsservice.AssertLogsCount(1),
	)
	assert.NoError(t, err)
}

// TestJournaldPriorityFiltering tests filtering by log priority
func TestJournaldPriorityFiltering(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}

	logGroup := fmt.Sprintf("/aws/cloudwatch-agent/journald/%s", instanceId)
	logStream := instanceId

	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	common.CopyFile("resources/config_journald_priority.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)

	time.Sleep(sleepForFlush)

	// Write info level log (should be collected)
	infoMsg := fmt.Sprintf("Info level message at %s", time.Now().Format(time.RFC3339))
	common.RunCommand(fmt.Sprintf("echo '%s' | systemd-cat -t %s -p info", infoMsg, testIdentifier))

	// Write debug level log (should be filtered out by priority=info)
	debugMsg := fmt.Sprintf("Debug level message at %s", time.Now().Format(time.RFC3339))
	common.RunCommand(fmt.Sprintf("echo '%s' | systemd-cat -t %s -p debug", debugMsg, testIdentifier))

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	// Should only see 1 log (info level, debug filtered out)
	err := awsservice.ValidateLogs(
		logGroup,
		logStream,
		&start,
		&end,
		awsservice.AssertLogsCount(1),
	)
	assert.NoError(t, err)
}
