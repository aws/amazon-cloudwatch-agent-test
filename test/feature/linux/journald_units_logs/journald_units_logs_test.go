// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_units_logs

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

func TestJournaldUnitsLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Wait for journald receiver to initialize
	time.Sleep(10 * time.Second)

	// Generate sshd activity by triggering SSH connections
	for i := 0; i < 3; i++ {
		exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes", "localhost", "echo", "test").CombinedOutput()
		time.Sleep(2 * time.Second)
	}

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()

	err = awsservice.ValidateLogs(
		instanceId,
		"journald",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message
				if !strings.Contains(message, "\"_SYSTEMD_UNIT\":\"sshd.service\"") {
					return fmt.Errorf("found entry not from sshd.service unit: %.100s", message)
				}
			}
			log.Printf("All logs validated: %d entries, all matched!", len(events))
			return nil
		},
	)
	assert.NoError(t, err)
}
