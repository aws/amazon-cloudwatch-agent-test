// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_priority_logs

import (
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestJournaldPriorityLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Generate journald entries at different priority levels
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "Journald info priority log"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "err", "Journald err priority log"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "warning", "Journald warning priority log"))

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()
	log.Printf("Validating journald priority logs for instance %s", instanceId)

	// Validate err stream (should have err and above)
	err = awsservice.ValidateLogs(
		instanceId,
		"journald-err",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err)
}
