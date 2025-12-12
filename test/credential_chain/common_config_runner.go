// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package credential_chain

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/credential_chain/util"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

// CommonConfigTestRunner validates that the common-config.toml configuration takes precedence over the standard
// AWS SDK fallback.
type CommonConfigTestRunner struct {
	test_runner.BaseTestRunner
	accessKeyID string
}

var _ test_runner.ITestRunner = (*CommonConfigTestRunner)(nil)

// SetupBeforeAgentRun overrides BaseTestRunner to set up shared credentials file and common-config.toml
func (t *CommonConfigTestRunner) SetupBeforeAgentRun() error {
	log.Println("Setting up common config test...")

	common.RecreateAgentLogfile(common.AgentLogFile)

	// Get AssumeRole ARN from environment metadata
	roleArn := metadata.AssumeRoleArn
	if roleArn == "" {
		return fmt.Errorf("AssumeRoleArn not provided in environment metadata")
	}

	creds, err := awsservice.GetCredentials(roleArn, t.GetTestName(), awsservice.DefaultAssumeRoleDuration)
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}

	// Store access key ID for validation
	t.accessKeyID = *creds.AccessKeyId

	log.Println("Successfully assumed role")

	overrideCredentialsProfile := "test-profile"
	overrideCredentialsPath := filepath.Join("/tmp", util.AwsCredentialsPath)
	// Create shared credentials file
	if err = util.SetupSharedCredentialsFile(
		filepath.Join("agent_configs", "credentials"),
		overrideCredentialsProfile,
		*creds.AccessKeyId,
		*creds.SecretAccessKey,
		*creds.SessionToken,
		overrideCredentialsPath,
	); err != nil {
		return fmt.Errorf("failed to create credentials file: %w", err)
	}
	t.RegisterCleanup(func() error {
		return util.CleanupCredentialsFile(overrideCredentialsPath)
	})

	log.Printf("Created credentials file at %s with profile %s", overrideCredentialsPath, overrideCredentialsProfile)

	// Create common-config.toml
	if err = util.SetupCommonConfig(filepath.Join("agent_configs", "common-config.toml"), overrideCredentialsProfile, overrideCredentialsPath); err != nil {
		// Clean up credentials file on failure
		return fmt.Errorf("failed to create common-config.toml: %w", errors.Join(err, util.CleanupCredentialsFile(overrideCredentialsPath)))
	}
	t.RegisterCleanup(util.ResetCommonConfig)

	log.Println("Created common-config.toml with shared credentials configuration")

	// Setup invalid credentials at user home to validate common-config takes higher priority
	userCredentialsPath := filepath.Join(util.UserCWAgent, util.AwsCredentialsPath)
	if err = util.SetupSharedCredentialsFile(
		filepath.Join("agent_configs", "credentials"),
		util.DefaultProfile,
		"invalidAccessKey",
		"invalidSecretKey",
		"invalidSessionToken",
		userCredentialsPath,
	); err != nil {
		return fmt.Errorf("failed to create user credentials file: %w", err)
	}
	t.RegisterCleanup(func() error {
		return util.CleanupCredentialsFile(userCredentialsPath)
	})

	return t.SetUpConfig()
}

// SetUpConfig overrides BaseTestRunner to handle namespace replacement
func (t *CommonConfigTestRunner) SetUpConfig() error {
	return util.SetupAgentConfig(t.GetAgentConfigFileName(), util.SharedTestNamespace, t.GetTestName(), util.UserCWAgent)
}

// Validate verifies that metrics were sent using shared credentials
func (t *CommonConfigTestRunner) Validate() status.TestGroupResult {
	t.Cleanup()
	return util.ValidateCredentialTest(t.GetTestName(), util.ExpectedResults{
		Namespace:              util.SharedTestNamespace,
		MetricName:             util.MetricNameCpuUsageActive,
		CredentialProviderName: util.ProviderNameSharedCredentials,
		AccessKeyID:            t.accessKeyID,
	}, metadata)
}

// GetTestName returns the test name
func (t *CommonConfigTestRunner) GetTestName() string {
	return "CommonConfigTest"
}

// GetAgentConfigFileName returns the agent config file name
func (t *CommonConfigTestRunner) GetAgentConfigFileName() string {
	return filepath.Join("agent_configs", "minimal_config.json")
}

// GetMeasuredMetrics returns the metrics to measure
func (t *CommonConfigTestRunner) GetMeasuredMetrics() []string {
	return util.MeasuredMetrics
}

func (t *CommonConfigTestRunner) GetAgentRunDuration() time.Duration {
	return util.DefaultAgentRunTime
}
