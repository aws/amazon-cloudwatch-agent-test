// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package credential_chain

import (
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

// HomeEnvTestRunner validates that the os.UserHomeDir (which evaluates to the HOME environment variable on linux)
// takes precedence over the user.Current().HomeDir. This is to support backwards compatibility. This is only relevant
// for the root user as any non-root run_as_user will set the HOME environment variable to the HomeDir during the
// switchUser on startup (https://github.com/aws/amazon-cloudwatch-agent/blob/65fb1dfaf31890487b2a6f7f1b53c1a52af3d14a/internal/util/user/userutil_linux.go#L126)
type HomeEnvTestRunner struct {
	test_runner.BaseTestRunner
	accessKeyID string
}

var _ test_runner.ITestRunner = (*HomeEnvTestRunner)(nil)

// SetupBeforeAgentRun sets up backwards compatibility home directory resolution test
func (t *HomeEnvTestRunner) SetupBeforeAgentRun() error {
	log.Println("Setting up home directory backwards compatibility test...")

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

	// Create credentials file at custom HOME location (/tmp/test-home/.aws/credentials) with valid credentials
	overrideCredentialsPath := filepath.Join(util.DefaultOverrideHomeDir, util.AwsCredentialsPath)
	overrideDir := filepath.Dir(overrideCredentialsPath)
	if err = common.MkdirAll(overrideDir); err != nil {
		return fmt.Errorf("failed to create custom home credentials directory: %w", err)
	}

	if err = util.SetupSharedCredentialsFile(
		filepath.Join("agent_configs", "credentials"),
		util.DefaultProfile,
		*creds.AccessKeyId,
		*creds.SecretAccessKey,
		*creds.SessionToken,
		overrideCredentialsPath,
	); err != nil {
		return fmt.Errorf("failed to create custom home credentials file: %w", err)
	}
	t.RegisterCleanup(func() error {
		return util.CleanupCredentialsFile(overrideCredentialsPath)
	})

	log.Printf("Created credentials file at override HOME location: %s", overrideCredentialsPath)

	// Set HOME environment variable in systemd override to custom path
	overrideContent := fmt.Sprintf("[Service]\nEnvironment=\"HOME=%s\"\n", util.DefaultOverrideHomeDir)
	if err = util.SetupSystemdOverride(overrideContent); err != nil {
		return fmt.Errorf("failed to setup systemd environment: %w", err)
	}
	t.RegisterCleanup(func() error {
		if err = util.CleanupSystemdOverride(); err != nil {
			return err
		}
		return util.ReloadSystemd()
	})

	// Reload systemd to apply changes
	if err = util.ReloadSystemd(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	log.Printf("Set HOME environment variable to %s in systemd override", util.DefaultOverrideHomeDir)

	// Empty common-config (force fallback to AWS SDK default chain)
	if err = util.ResetCommonConfig(); err != nil {
		return fmt.Errorf("failed to reset common config: %w", err)
	}

	log.Println("Set empty common-config.toml to force AWS SDK default chain")

	// Setup invalid credentials at user default home to validate HOME env takes higher priority
	rootCredentialsPath := filepath.Join(util.UserRootHomeDir, util.AwsCredentialsPath)
	if err = util.SetupSharedCredentialsFile(
		filepath.Join("agent_configs", "credentials"),
		util.DefaultProfile,
		"invalidAccessKey",
		"invalidSecretKey",
		"invalidSessionToken",
		rootCredentialsPath,
	); err != nil {
		return fmt.Errorf("failed to create root credentials file: %w", err)
	}
	t.RegisterCleanup(func() error {
		return util.CleanupCredentialsFile(rootCredentialsPath)
	})

	return t.SetUpConfig()
}

// SetUpConfig overrides BaseTestRunner to handle namespace replacement
func (t *HomeEnvTestRunner) SetUpConfig() error {
	return util.SetupAgentConfig(t.GetAgentConfigFileName(), util.SharedTestNamespace, t.GetTestName(), util.UserRoot)
}

// Validate verifies that metrics were sent using backwards compatible home directory resolution
func (t *HomeEnvTestRunner) Validate() status.TestGroupResult {
	// need to clean up the invalid root credentials before validation runs
	t.Cleanup()
	return util.ValidateCredentialTest(t.GetTestName(), util.ExpectedResults{
		Namespace:              util.SharedTestNamespace,
		MetricName:             util.MetricNameCpuUsageActive,
		CredentialProviderName: util.ProviderNameSharedConfig,
		AccessKeyID:            t.accessKeyID,
	}, metadata)
}

// GetTestName returns the test name
func (t *HomeEnvTestRunner) GetTestName() string {
	return "HomeEnvTest"
}

// GetAgentConfigFileName returns the agent config file name
func (t *HomeEnvTestRunner) GetAgentConfigFileName() string {
	return filepath.Join("agent_configs", "minimal_config.json")
}

// GetMeasuredMetrics returns the metrics to measure
func (t *HomeEnvTestRunner) GetMeasuredMetrics() []string {
	return util.MeasuredMetrics
}

func (t *HomeEnvTestRunner) GetAgentRunDuration() time.Duration {
	return util.DefaultAgentRunTime
}
