// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_regex_logs

import (
	"fmt"
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

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()

	// Validate: only entries matching the include regex should appear
	err = awsservice.ValidateLogs(
		instanceId,
		"journald-regex",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
		func(events []types.OutputLogEvent) error {
			for _, event := range events {
				message := *event.Message
				// Every entry should match the include pattern
				if !strings.Contains(message, "Did not receive") {
					return fmt.Errorf("found entry not matching include regex: %.100s", message)
				}
			}
			return nil
		},
	)
	assert.NoError(t, err)
}
