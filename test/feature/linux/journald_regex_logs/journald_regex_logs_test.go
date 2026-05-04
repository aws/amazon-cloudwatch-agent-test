// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_regex_logs

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

func TestJournaldRegexLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Generate journald entries that match/don't match the regex filters
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "err", "Database connection failed"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "warning", "Authentication failed for user admin"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "user login successful"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "Service started successfully"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "CWAgent supports regex"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "CWAgent has stopped"))

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()
	log.Printf("Validating journald regex logs for instance %s", instanceId)

	// Validate regex1 stream (include: Database.*failed|Authentication.*|login.*)
	err = awsservice.ValidateLogs(
		instanceId,
		"journald-regex1",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err)
}
