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
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	recoveryPolicyPrefix = "cwagent-recovery-deny-"
	iamPropagationWait   = 30 * time.Second
)

// TestConcurrencyRecovery validates that the agent recovers and publishes logs
// after IAM deny permissions are removed mid-test.
func TestConcurrencyRecovery(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}

	iamRoleName := env.IamRoleName
	if iamRoleName == "" {
		t.Skip("Skipping TestConcurrencyRecovery: -iamRoleName not provided (requires per-test IAM role with self-modify permissions)")
	}

	allowedLogGroup := fmt.Sprintf("recovery-allowed-%s", instanceId)
	recoveryLogGroup := fmt.Sprintf("recovery-test-target-%s", instanceId)
	policyName := recoveryPolicyPrefix + instanceId
	logGroupArn := fmt.Sprintf("arn:aws:logs:*:*:log-group:%s:*", recoveryLogGroup)

	err := awsservice.PutRoleDenyPolicy(iamRoleName, policyName, logGroupArn)
	require.NoError(t, err, "Failed to create deny policy")

	policyCreated := true
	defer func() {
		if policyCreated {
			if cleanupErr := awsservice.DeleteRoleInlinePolicy(iamRoleName, policyName); cleanupErr != nil {
				t.Logf("Warning: failed to cleanup deny policy: %v", cleanupErr)
			}
		}
	}()

	time.Sleep(iamPropagationWait)

	defer awsservice.DeleteLogGroupAndStream(allowedLogGroup, instanceId)
	defer awsservice.DeleteLogGroupAndStream(recoveryLogGroup, instanceId)

	allowedFile, err := os.Create("/tmp/recovery_allowed.log")
	require.NoError(t, err)
	defer allowedFile.Close()
	defer os.Remove("/tmp/recovery_allowed.log")

	recoveryFile, err := os.Create("/tmp/recovery_target.log")
	require.NoError(t, err)
	defer recoveryFile.Close()
	defer os.Remove("/tmp/recovery_target.log")

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)

	common.CopyFile("resources/config_recovery.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)

	start := time.Now()
	writeLogLines(t, allowedFile, 10)
	writeLogLines(t, recoveryFile, 10)
	time.Sleep(sleepForFlush)
	phase1End := time.Now()

	err = awsservice.ValidateLogs(allowedLogGroup, instanceId, &start, &phase1End,
		awsservice.AssertLogsCount(20))
	assert.NoError(t, err, "Allowed log group should have logs")

	err = awsservice.ValidateLogs(recoveryLogGroup, instanceId, &start, &phase1End,
		awsservice.AssertLogsCount(0))
	assert.Error(t, err, "Recovery log group should not exist while denied")
	assert.Contains(t, err.Error(), "ResourceNotFoundException")

	err = awsservice.DeleteRoleInlinePolicy(iamRoleName, policyName)
	assert.NoError(t, err, "Failed to delete deny policy")
	policyCreated = false

	t.Logf("Deny policy removed, waiting %v for IAM propagation...", iamPropagationWait)
	time.Sleep(iamPropagationWait)

	recoveryStart := time.Now()
	writeLogLines(t, recoveryFile, 10)
	time.Sleep(sleepForFlush)

	common.StopAgent()
	end := time.Now()

	err = awsservice.ValidateLogs(recoveryLogGroup, instanceId, &recoveryStart, &end,
		awsservice.AssertLogsCount(20))
	if err != nil {
		printAgentLogs(t)
	}
	assert.NoError(t, err, "Recovery log group should have logs after permissions restored")
}

func printAgentLogs(t *testing.T) {
	t.Log("=== CloudWatch Agent Logs (last 100 lines) ===")
	content, err := os.ReadFile(common.AgentLogFile)
	if err != nil {
		t.Logf("Failed to read agent log: %v", err)
		return
	}
	lines := string(content)
	// Print last ~100 lines
	lineSlice := splitLines(lines)
	start := 0
	if len(lineSlice) > 100 {
		start = len(lineSlice) - 100
	}
	for i := start; i < len(lineSlice); i++ {
		t.Log(lineSlice[i])
	}
	t.Log("=== End Agent Logs ===")
}

func splitLines(s string) []string {
	var lines []string
	for len(s) > 0 {
		idx := 0
		for idx < len(s) && s[idx] != '\n' {
			idx++
		}
		lines = append(lines, s[:idx])
		if idx < len(s) {
			s = s[idx+1:]
		} else {
			break
		}
	}
	return lines
}
