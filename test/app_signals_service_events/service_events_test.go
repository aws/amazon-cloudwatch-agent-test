// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Startup tests for the Application Signals (Telemend / service-events)
// pipeline, covering credential resolution and custom CA bundle handling.
// After confirming the agent starts, each test runs a basic ServiceEvents
// logs + metrics end-to-end check.
//
//   - TestAppSignalsOnPremCredentialsStartup: the agent starts in onPrem mode
//     using a credentials file even when IMDS is unreachable (the sigv4auth
//     extension must use the provided credentials rather than resolving via the
//     SDK default chain, which hits IMDS).
//
//   - TestAppSignalsCustomCABundleStartup: the agent starts when AWS_CA_BUNDLE
//     is set, exercising the awscloudwatchlogsprovisioner extension's SDK client
//     accepting custom root CAs (required in ISO/ADC/ITAR partitions).
package app_signals_service_events

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	credutil "github.com/aws/amazon-cloudwatch-agent-test/test/credential_chain/util"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	onPremCredsDir           = "/tmp/.aws"
	onPremCredsFile          = onPremCredsDir + "/credentials"
	caBundlePath             = "/tmp/cwagent-ca-bundle.pem"
	untrustedCABundlePath    = "/tmp/cwagent-untrusted-ca-bundle.pem"
	systemCABundlePath       = "/etc/pki/tls/certs/ca-bundle.crt"
	agentCtl                 = "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl"
	credsTemplatePath        = "agent_configs/credentials"
	commonConfigTemplatePath = "agent_configs/common-config.toml"
)

// agentStatus is the JSON shape returned by `amazon-cloudwatch-agent-ctl -a status`.
type agentStatus struct {
	Status       string `json:"status"`
	StartTime    string `json:"starttime"`
	ConfigStatus string `json:"configstatus"`
	Version      string `json:"version"`
}

// getAgentStatus returns the parsed agent status from the control script.
func getAgentStatus(t *testing.T) agentStatus {
	t.Helper()
	out, err := common.RunCommand("sudo " + agentCtl + " -a status")
	require.NoError(t, err, "Failed to get agent status")
	var s agentStatus
	require.NoError(t, json.Unmarshal([]byte(out), &s), "Failed to parse agent status: %s", out)
	return s
}

// assertAgentStable verifies the agent is running and stays running with the
// same start time across the polling window. The agent is started via systemd,
// which restarts on failure — a crash-looping agent can momentarily report
// "running", so we require a stable start time (no restart) to confirm it is
// genuinely up rather than caught between restarts.
func assertAgentStable(t *testing.T, msgAndArgs ...any) {
	t.Helper()
	first := getAgentStatus(t)
	require.Equal(t, "running", first.Status, msgAndArgs...)
	for i := 0; i < 3; i++ {
		time.Sleep(5 * time.Second)
		s := getAgentStatus(t)
		require.Equal(t, "running", s.Status, msgAndArgs...)
		require.Equal(t, first.StartTime, s.StartTime,
			"agent restarted (start time changed from %q to %q) — not stable", first.StartTime, s.StartTime)
	}
}

// assertServiceEventsE2E sends ServiceEvents logs and metrics through the running
// agent and verifies both pipelines deliver: logs to a dynamic CW log group, and
// metrics to the OTLP monitoring endpoint (queryable via PromQL).
func assertServiceEventsE2E(t *testing.T, serviceName string) {
	t.Helper()
	logGroup := logGroupPrefix + serviceName
	numLogs := 5
	defer awsservice.DeleteLogGroupAndStream(logGroup, testStreamName)

	start := time.Now()
	sendOTLPLogs(t, serviceName, numLogs)
	sendMetrics(t, "service_events_metric", 3, "ServiceEvents")
	time.Sleep(sleepForFlush)
	end := time.Now()

	// Logs pipeline: logs land in the dynamic per-service log group.
	err := awsservice.ValidateLogs(
		logGroup, testStreamName, &start, &end,
		awsservice.AssertLogsCount(numLogs),
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err, "ServiceEvents logs should be delivered to %s", logGroup)

	// Metrics pipeline: ServiceEvents metric is queryable via the OTLP endpoint.
	result := otlpvalidation.ValidateOtlpMetrics(
		"service_events_startup", "us-west-2", []string{"service_events_metric"},
	)
	for _, r := range result.TestResults {
		assert.Equal(t, status.SUCCESSFUL, r.Status,
			"ServiceEvents metric %s should be queryable via CW OTLP PromQL API: %v", r.Name, r.Reason)
	}
}

// onPremiseStartCommand starts the agent in onPremise mode via the agent control
// script (common.StartAgent defaults to ec2 mode).
const onPremiseStartCommand = "sudo " + agentCtl + " -a fetch-config -m onPremise -s -c "

// disableIMDS sets AWS_EC2_METADATA_DISABLED=true in the agent's systemd
// environment so the agent cannot resolve credentials from IMDS at runtime,
// forcing it to use the provided credentials file. Returns a cleanup func.
func disableIMDS(t *testing.T) func() {
	t.Helper()
	require.NoError(t, credutil.SetupSystemdOverride("[Service]\nEnvironment=\"AWS_EC2_METADATA_DISABLED=true\"\n"),
		"Failed to set systemd override")
	require.NoError(t, credutil.ReloadSystemd(), "Failed to reload systemd")
	return func() {
		_ = credutil.CleanupSystemdOverride()
		_ = credutil.ReloadSystemd()
	}
}

// TestAppSignalsOnPremCredentialsStartup verifies the agent starts up in onPrem
// mode using a credentials file, even when IMDS is unreachable, and delivers
// ServiceEvents logs and metrics.
func TestAppSignalsOnPremCredentialsStartup(t *testing.T) {
	common.RecreateAgentLogfile(common.AgentLogFile)

	// Resolve the instance-role credentials (via the SDK default chain → IMDS) and
	// write them to a shared credentials file so the agent has credentials from
	// the file alone once IMDS is disabled.
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-west-2"))
	require.NoError(t, err, "Failed to load AWS config")
	creds, err := cfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err, "Failed to resolve instance credentials")
	require.NoError(t, common.MkdirAll(onPremCredsDir), "Failed to create credentials dir")
	require.NoError(t, credutil.SetupSharedCredentialsFile(
		credsTemplatePath, credutil.DefaultProfile,
		creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, onPremCredsFile,
	), "Failed to write credentials file")
	// Region for onPrem mode is read from a config file next to the credentials file.
	require.NoError(t, common.WriteFile(onPremCredsDir+"/config", "[default]\nregion = us-west-2\n"),
		"Failed to write region config file")

	// common-config.toml points the agent at the credentials file.
	require.NoError(t, credutil.SetupCommonConfig(
		commonConfigTemplatePath, credutil.DefaultProfile, onPremCredsFile),
		"Failed to write common-config.toml")
	defer credutil.ResetCommonConfig()

	common.CopyFile(logsConfigPath, common.ConfigOutputPath)

	// Disable IMDS for the agent so it must use the credentials file. This only
	// affects the agent's systemd environment, not the test process, so the e2e
	// validation's own AWS SDK calls (GetLogEvents, PromQL) still work.
	defer disableIMDS(t)()
	defer common.StopAgent()

	// Start the agent in onPremise mode. sigv4 credential resolution should use
	// the provided credentials file rather than the SDK default chain (IMDS).
	common.StartAgentWithCommand(common.ConfigOutputPath, false, false, onPremiseStartCommand)
	time.Sleep(10 * time.Second)

	agentLog := common.ReadAgentLogfile(common.AgentLogFile)
	assert.NotContains(t, agentLog, "could not retrieve credential provider",
		"sigv4auth should not eagerly resolve credentials via IMDS when a credentials file is provided")
	assert.NotContains(t, agentLog, "no EC2 IMDS role found",
		"sigv4auth should use the provided credentials file instead of requiring IMDS")

	assertAgentStable(t,
		"agent should start in onPrem mode with a credentials file even when IMDS is disabled")

	assertServiceEventsE2E(t, "onprem-creds-svc")
}

// startAgentWithCABundle points common-config.toml at the given CA bundle (so the
// agent runs with AWS_CA_BUNDLE set), starts the agent, and waits for it to settle.
func startAgentWithCABundle(t *testing.T, bundlePath string) {
	t.Helper()
	common.RecreateAgentLogfile(common.AgentLogFile)

	// [ssl] ca_bundle_path → ctl emits AWS_CA_BUNDLE into env-config.json, which
	// the agent service picks up.
	require.NoError(t, common.WriteFile(common.AgentCommonConfigFile,
		"[ssl]\n  ca_bundle_path = \""+bundlePath+"\"\n"),
		"Failed to write common-config.toml")
	common.CopyFile(logsConfigPath, common.ConfigOutputPath)
	common.StartAgent(common.ConfigOutputPath, false, false)
	time.Sleep(10 * time.Second)
}

// TestAppSignalsCustomCABundleStartup verifies the awscloudwatchlogsprovisioner
// extension honors a custom AWS_CA_BUNDLE. The extension must build its SDK
// client with a BuildableClient so custom root CAs can be injected; a plain
// *http.Client cannot, which previously crashed the agent on startup.
func TestAppSignalsCustomCABundleStartup(t *testing.T) {
	defer common.StopAgent()
	defer credutil.ResetCommonConfig()

	// A valid CA bundle (copy of the system store) lets the agent start, accept
	// the custom root CAs, and export ServiceEvents data end-to-end.
	t.Run("valid_ca_bundle_exports_data", func(t *testing.T) {
		require.NoError(t, common.RunCommands([]string{
			fmt.Sprintf("sudo cp %s %s", systemCABundlePath, caBundlePath),
		}), "Failed to copy CA bundle")

		startAgentWithCABundle(t, caBundlePath)

		agentLog := common.ReadAgentLogfile(common.AgentLogFile)
		assert.NotContains(t, agentLog, "failed to create CW Logs client",
			"provisioner extension should build an SDK client that supports custom root CAs")
		assert.NotContains(t, agentLog, "Error running agent",
			"agent should not fail to start with a custom AWS_CA_BUNDLE set")

		assertAgentStable(t, "agent should start with a valid custom AWS_CA_BUNDLE")
		assertServiceEventsE2E(t, "ca-bundle-svc")
	})

	// A standalone self-signed bundle (which does NOT trust the real AWS CAs)
	// proves the custom bundle is actually consulted: the agent still starts (the
	// provisioner builds its client without crashing), but its outbound TLS to AWS
	// fails with x509 — which only happens if the agent loaded OUR bundle instead
	// of the system trust store. This is the strong signal that AWS_CA_BUNDLE is
	// honored, not silently ignored.
	t.Run("custom_ca_bundle_is_honored", func(t *testing.T) {
		common.StopAgent()
		generateSelfSignedBundle(t, untrustedCABundlePath)

		startAgentWithCABundle(t, untrustedCABundlePath)

		// Provisioner builds its client without crashing the agent...
		assertAgentStable(t, "agent should start even with an untrusted custom AWS_CA_BUNDLE")

		// ...and the custom (untrusted) bundle is actually used for outbound TLS.
		sendOTLPLogs(t, "ca-bundle-untrusted-svc", 1)
		time.Sleep(sleepForFlush)
		common.StopAgent()
		agentLog := common.ReadAgentLogfile(common.AgentLogFile)
		assert.Contains(t, agentLog, "x509: certificate signed by unknown authority",
			"agent should use the custom (untrusted) CA bundle for outbound TLS, proving AWS_CA_BUNDLE is honored")
	})
}

// generateSelfSignedBundle writes a standalone self-signed certificate to path.
// It is a valid PEM bundle but does not contain any real AWS root CAs.
func generateSelfSignedBundle(t *testing.T, path string) {
	t.Helper()
	cmd := fmt.Sprintf("sudo openssl req -x509 -newkey rsa:2048 -nodes -keyout /dev/null "+
		"-subj '/CN=cwagent-test-untrusted' -days 1 -out %s", path)
	_, err := common.RunCommand(cmd)
	require.NoError(t, err, "Failed to generate self-signed CA bundle")
}
