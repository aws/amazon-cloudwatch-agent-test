// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_matches_logs

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

func TestJournaldMatchesLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Generate entry that matches the SYSLOG_IDENTIFIER filter
	assert.NoError(t, common.CreateJournaldEntry("cwagent-match-test", "info", "Matched journald entry"))

	// Generate entry with different identifier (should NOT be collected)
	assert.NoError(t, common.CreateJournaldEntry("cwagent-no-match", "info", "This should not appear"))

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()
	log.Printf("Validating journald matches logs for instance %s", instanceId)

	err = awsservice.ValidateLogs(
		instanceId,
		"journald-matches",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err)
}
