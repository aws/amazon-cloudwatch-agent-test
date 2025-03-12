// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package selinux_negative_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestSelinuxNegativeTest(t *testing.T) {
	logGroupName, workingLogGroupName := startAgent(t)
	time.Sleep(2 * time.Minute)
	verifyLogStreamDoesExist(t, workingLogGroupName) // This should have a log stream
	verifyLogStreamDoesNotExist(t, logGroupName)     // This should not have a log stream
}

func startAgent(t *testing.T) (string, string) {
	randomNumber := rand.Int63()
	logGroupName := fmt.Sprintf("/aws/cloudwatch/shadow-%d", randomNumber)
	workingLogGroupName := fmt.Sprintf("/aws/cloudwatch/working-%d", randomNumber)

	configFilePath := filepath.Join("agent_configs", "config.json")

	originalConfigContent, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	updatedConfigContent := strings.ReplaceAll(string(originalConfigContent), "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)

	err = os.WriteFile(configFilePath, []byte(updatedConfigContent), os.ModePerm)
	require.NoError(t, err)
	require.NoError(t, common.StartAgent(configFilePath, true, false))

	err = os.WriteFile(configFilePath, originalConfigContent, os.ModePerm)
	require.NoError(t, err)
	return logGroupName, workingLogGroupName
}

func verifyLogStreamDoesNotExist(t *testing.T, logGroupName string) {
	logStreams := awsservice.GetLogStreamNames(logGroupName)
	require.Len(t, logStreams, 0)
}

func verifyLogStreamDoesExist(t *testing.T, logGroupName string) {
	logStreams := awsservice.GetLogStreamNames(logGroupName)
	require.Greater(t, len(logStreams), 0)
}
