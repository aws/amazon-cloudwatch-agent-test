// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald_units_logs

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

func TestJournaldUnitsLogs(t *testing.T) {
	common.CopyFile("agent_config.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	assert.NoError(t, err)

	// Generate journald entries
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "info", "Journald info log"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "warning", "Journald warning log"))
	assert.NoError(t, common.CreateJournaldEntry("cwagent-journald-test", "err", "Journald err log"))

	time.Sleep(60 * time.Second)
	common.StopAgent()

	instanceId := awsservice.GetInstanceId()
	log.Printf("Validating journald unit logs for instance %s", instanceId)

	err = awsservice.ValidateLogs(
		instanceId,
		"journald",
		nil,
		nil,
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err)
}
