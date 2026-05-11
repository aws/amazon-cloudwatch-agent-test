// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_regex_logs

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

func TestJournaldRegexLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(10 * time.Second)

	// Generate entries that MATCH the include filter: ".*error.*|.*failed.*"
	exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.err", "Database connection error occurred").Run()
	exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.err", "Authentication failed for user").Run()
	// Generate entries that should NOT match the filter
	exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.info", "Service started successfully").Run()
	exec.Command("logger", "-t", "cwagent-regex-test", "-p", "user.info", "Health check passed").Run()

	time.Sleep(60 * time.Second)
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
