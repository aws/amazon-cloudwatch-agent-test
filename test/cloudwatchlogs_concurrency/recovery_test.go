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
	recoveryPolicyName = "cwagent-recovery-test-deny"
	logGroupPattern    = "arn:aws:logs:*:*:log-group:recovery-test-*:*"
	iamRoleName        = "cwa-e2e-iam-role"
	iamPropagationWait = 30 * time.Second
)

// TestConcurrencyRecovery validates that the agent recovers and publishes logs
// after IAM deny permissions are removed mid-test.
func TestConcurrencyRecovery(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	instanceId := env.InstanceId
	if instanceId == "" {
		instanceId = awsservice.GetInstanceId()
	}

	// Create inline deny policy (separate from the static Terraform-managed deny)
	err := awsservice.PutRoleDenyPolicy(iamRoleName, recoveryPolicyName, logGroupPattern)
	require.NoError(t, err, "Failed to create deny policy")

	policyCreated := true
	defer func() {
		if policyCreated {
			if cleanupErr := awsservice.DeleteRoleInlinePolicy(iamRoleName, recoveryPolicyName); cleanupErr != nil {
				t.Logf("Warning: failed to cleanup deny policy: %v", cleanupErr)
			}
		}
	}()

	time.Sleep(iamPropagationWait)

	allowedLogGroup := fmt.Sprintf("recovery-allowed-%s", instanceId)
	recoveryLogGroup := fmt.Sprintf("recovery-test-target-%s", instanceId)

	defer awsservice.DeleteLogGroupAndStream(allowedLogGroup, instanceId)
	defer awsservice.DeleteLogGroupAndStream(recoveryLogGroup, instanceId)

	// Create log files
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

	// Phase 1 — Write while denied
	start := time.Now()
	writeLogLines(t, allowedFile, 10)
	writeLogLines(t, recoveryFile, 10)
	time.Sleep(sleepForFlush)
	phase1End := time.Now()

	// Verify allowed group has logs
	err = awsservice.ValidateLogs(allowedLogGroup, instanceId, &start, &phase1End,
		awsservice.AssertLogsCount(20))
	assert.NoError(t, err, "Allowed log group should have logs")

	// Verify recovery group does NOT have logs
	err = awsservice.ValidateLogs(recoveryLogGroup, instanceId, &start, &phase1End,
		awsservice.AssertLogsCount(0))
	assert.Error(t, err, "Recovery log group should not exist while denied")
	assert.Contains(t, err.Error(), "ResourceNotFoundException")

	// Phase 2 — Remove deny policy to grant permission
	err = awsservice.DeleteRoleInlinePolicy(iamRoleName, recoveryPolicyName)
	assert.NoError(t, err, "Failed to delete deny policy")
	policyCreated = false

	t.Logf("Deny policy removed, waiting %v for IAM propagation...", iamPropagationWait)
	time.Sleep(iamPropagationWait)

	// Phase 3 — Write more logs after permission restored
	recoveryStart := time.Now()
	writeLogLines(t, recoveryFile, 10)
	time.Sleep(sleepForFlush)

	common.StopAgent()
	end := time.Now()

	// Phase 4 — Verify recovery group now has logs
	err = awsservice.ValidateLogs(recoveryLogGroup, instanceId, &recoveryStart, &end,
		awsservice.AssertLogsCount(20))
	assert.NoError(t, err, "Recovery log group should have logs after permissions restored")
}
