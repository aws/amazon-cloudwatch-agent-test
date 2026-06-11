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
package app_signals

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otlp_export/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	imdsEndpoint       = "169.254.169.254"
	onPremCredsDir     = "/tmp/.aws"
	onPremCredsFile    = onPremCredsDir + "/credentials"
	caBundlePath       = "/tmp/cwagent-ca-bundle.pem"
	systemCABundlePath = "/etc/pki/tls/certs/ca-bundle.crt"
	commonConfigOutput = "/opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"
	agentCtl           = "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl"
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
		"service_events_startup", "us-east-1", []string{"service_events_metric"},
	)
	for _, r := range result.TestResults {
		assert.Equal(t, status.SUCCESSFUL, r.Status,
			"ServiceEvents metric %s should be queryable via CW OTLP PromQL API: %v", r.Name, r.Reason)
	}
}

// fetchConfig translates the given config and starts the agent, returning the
// combined command output.
func fetchConfig(t *testing.T, mode string) string {
	t.Helper()
	out, _ := common.RunCommand(fmt.Sprintf(
		"sudo %s -a fetch-config -m %s -s -c file:%s", agentCtl, mode, common.ConfigOutputPath))
	return out
}

// blockIMDS adds an iptables rule rejecting traffic to the IMDS endpoint so the
// AWS SDK default credential chain cannot resolve credentials from IMDS.
func blockIMDS(t *testing.T) {
	t.Helper()
	_, err := common.RunCommand(fmt.Sprintf("sudo iptables -A OUTPUT -d %s -j REJECT", imdsEndpoint))
	require.NoError(t, err, "Failed to block IMDS")
}

// unblockIMDS removes the iptables rule added by blockIMDS.
func unblockIMDS(t *testing.T) {
	t.Helper()
	_, _ = common.RunCommand(fmt.Sprintf("sudo iptables -D OUTPUT -d %s -j REJECT", imdsEndpoint))
}

// TestAppSignalsOnPremCredentialsStartup verifies the agent starts up in onPrem
// mode using a credentials file, even when IMDS is unreachable, and delivers
// ServiceEvents logs and metrics.
func TestAppSignalsOnPremCredentialsStartup(t *testing.T) {
	common.RecreateAgentLogfile(common.AgentLogFile)

	// Write a valid credentials file sourced from the instance role (via IMDSv2)
	// BEFORE blocking IMDS, so the agent has credentials from the file alone.
	writeCredsScript := `
set -e
mkdir -p ` + onPremCredsDir + `
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 600")
ROLE=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/iam/security-credentials/)
CREDS=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/iam/security-credentials/$ROLE)
AKID=$(echo "$CREDS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKeyId"])')
SAK=$(echo "$CREDS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["SecretAccessKey"])')
TOK=$(echo "$CREDS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Token"])')
printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\naws_session_token=%s\n' "$AKID" "$SAK" "$TOK" | sudo tee ` + onPremCredsFile + `>/dev/null
# Region for onPrem mode is read from a config file next to the credentials file.
printf '[default]\nregion = us-east-1\n' | sudo tee ` + onPremCredsDir + `/config>/dev/null`
	require.NoError(t, common.RunCommands([]string{writeCredsScript}), "Failed to write credentials file")

	// common-config.toml points the agent at the credentials file.
	require.NoError(t, common.RunCommands([]string{
		"printf '[credentials]\\n  shared_credential_profile = \"default\"\\n  shared_credential_file = \"" +
			onPremCredsFile + "\"\\n' | sudo tee " + commonConfigOutput + ">/dev/null",
	}), "Failed to write common-config.toml")

	common.CopyFile(logsConfigPath, common.ConfigOutputPath)

	blockIMDS(t)
	defer unblockIMDS(t)
	defer common.StopAgent()
	defer common.RunCommand("sudo " + agentCtl + " -a remove-config -c all")

	// Start the agent in onPremise mode. sigv4 credential resolution should use
	// the provided credentials file rather than the SDK default chain (IMDS).
	// Validation errors surface in the fetch-config output.
	out := fetchConfig(t, "onPremise")
	time.Sleep(10 * time.Second)

	assert.NotContains(t, out, "could not retrieve credential provider",
		"sigv4auth should not eagerly resolve credentials via IMDS when a credentials file is provided")
	assert.NotContains(t, out, "no EC2 IMDS role found",
		"sigv4auth should use the provided credentials file instead of requiring IMDS")

	assertAgentStable(t,
		"agent should start in onPrem mode with a credentials file even when IMDS is unreachable")

	// The agent started without IMDS (the behavior under test). Restore IMDS so the
	// e2e validation's own AWS SDK calls (GetLogEvents, PromQL) can authenticate.
	unblockIMDS(t)
	assertServiceEventsE2E(t, "onprem-creds-svc")
}

// TestAppSignalsCustomCABundleStartup verifies the agent starts up when
// AWS_CA_BUNDLE is set, exercising the awscloudwatchlogsprovisioner extension's
// need to inject custom root CAs into its SDK HTTP client.
func TestAppSignalsCustomCABundleStartup(t *testing.T) {
	common.RecreateAgentLogfile(common.AgentLogFile)

	// Use a copy of the system CA bundle as a custom AWS_CA_BUNDLE. This is a
	// valid bundle (so TLS still works), but its presence forces the SDK to
	// inject custom root CAs via WithTransportOptions — which a plain
	// *http.Client does not support.
	require.NoError(t, common.RunCommands([]string{
		fmt.Sprintf("sudo cp %s %s", systemCABundlePath, caBundlePath),
	}), "Failed to copy CA bundle")

	// common-config.toml sets [ssl] ca_bundle_path → ctl emits AWS_CA_BUNDLE into
	// env-config.json, which the agent service picks up.
	require.NoError(t, common.RunCommands([]string{
		"printf '[ssl]\\n  ca_bundle_path = \"" + caBundlePath + "\"\\n' | " +
			"sudo tee " + commonConfigOutput + ">/dev/null",
	}), "Failed to write common-config.toml")

	common.CopyFile(logsConfigPath, common.ConfigOutputPath)

	defer common.StopAgent()
	defer common.RunCommand("sudo " + agentCtl + " -a remove-config -c all")

	fetchConfig(t, "ec2")
	time.Sleep(10 * time.Second)

	// The provisioner extension must not fail to start due to custom root CAs.
	// (Scoped to fatal startup errors — a non-fatal RootCAs warning from other
	// components, e.g. ec2tagger, does not crash the agent.)
	agentLog := common.ReadAgentLogfile(common.AgentLogFile)
	assert.NotContains(t, agentLog, "failed to create CW Logs client",
		"provisioner extension should build an SDK client that supports custom root CAs")
	assert.NotContains(t, agentLog, "Error running agent",
		"agent should not fail to start with a custom AWS_CA_BUNDLE set")

	assertAgentStable(t,
		"agent should start with a custom AWS_CA_BUNDLE set")

	assertServiceEventsE2E(t, "ca-bundle-svc")
}
