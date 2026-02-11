// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs_concurrency

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	logLineId1       = "foo"
	logLineId2       = "bar"
	sleepForFlush    = 20 * time.Second
)

var logLineIds = []string{logLineId1, logLineId2}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestConcurrencyPoisonPill validates that when retry heap size equals concurrency
// and is smaller than the number of failing log groups, the allowed log group
// continues to publish logs despite multiple denied log groups.
func TestConcurrencyPoisonPill(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}

	accessGrantedLogGroup := fmt.Sprintf("access-granted-%s", instanceId)
	accessGrantedLogFile := "/tmp/access_granted.log"

	// Create 10 denied log groups
	deniedLogGroups := make([]string, 10)
	deniedLogFiles := make([]string, 10)
	for i := 0; i < 10; i++ {
		deniedLogGroups[i] = fmt.Sprintf("aws-restricted-log-group-name-%d-%s", i, instanceId)
		deniedLogFiles[i] = fmt.Sprintf("/tmp/access_denied_%d.log", i)
	}

	defer awsservice.DeleteLogGroupAndStream(accessGrantedLogGroup, instanceId)
	for _, lg := range deniedLogGroups {
		defer awsservice.DeleteLogGroupAndStream(lg, instanceId)
	}

	// Create log files
	grantedFile, err := os.Create(accessGrantedLogFile)
	assert.NoError(t, err)
	defer grantedFile.Close()
	defer os.Remove(accessGrantedLogFile)

	deniedFiles := make([]*os.File, 10)
	for i := 0; i < 10; i++ {
		deniedFiles[i], err = os.Create(deniedLogFiles[i])
		assert.NoError(t, err)
		defer deniedFiles[i].Close()
		defer os.Remove(deniedLogFiles[i])
	}

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	common.CopyFile("resources/config_concurrency.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)

	time.Sleep(sleepForFlush)

	// Write logs to all files
	writeLogLines(t, grantedFile, 10)
	for i := 0; i < 10; i++ {
		writeLogLines(t, deniedFiles[i], 10)
	}

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	// Validate access granted log group has logs
	err = awsservice.ValidateLogs(
		accessGrantedLogGroup,
		instanceId,
		&start,
		&end,
		awsservice.AssertLogsCount(20), // 10 iterations * 2 logLineIds
	)
	assert.NoError(t, err, "Access granted log group should have logs despite denied log groups")

	// Validate denied log groups have no logs
	for _, lg := range deniedLogGroups {
		err = awsservice.ValidateLogs(
			lg,
			instanceId,
			&start,
			&end,
			awsservice.AssertLogsCount(0),
		)
		assert.NoError(t, err, "Denied log group should have no logs")
	}

	// Check agent logs for access denied errors
	agentLog, err := os.ReadFile(common.AgentLogFile)
	if err == nil {
		logContent := string(agentLog)
		assert.Contains(t, logContent, "AccessDenied", "Agent logs should contain AccessDenied errors")
	}
}

func writeLogLines(t *testing.T, f *os.File, iterations int) {
	for i := 0; i < iterations; i++ {
		ts := time.Now()
		for _, id := range logLineIds {
			_, err := f.WriteString(fmt.Sprintf("%s - [%s] #%d This is a log line.\n", ts.Format(time.StampMilli), id, i))
			if err != nil {
				t.Logf("Error occurred writing log line: %v", err)
			}
		}
		time.Sleep(1 * time.Millisecond)
	}
}
