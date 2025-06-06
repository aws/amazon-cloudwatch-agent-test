// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package logfile

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestStateFile(t *testing.T) {
	err := Validate()
	if err != nil {
		content := common.ReadAgentLogfile(common.AgentLogFile)
		log.Printf("agent log file content:\n%s", content)
	}
	assert.NoError(t, err)
}
