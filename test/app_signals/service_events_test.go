// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Startup tests for the Application Signals (Telemend / service-events)
// pipeline, covering credential resolution and custom CA bundle handling.
//
//   - TestAppSignalsNoCredentialsStartup: the agent starts in onPrem mode using
//     a credentials file even when IMDS is unreachable (the sigv4auth extension
//     must use the provided credentials rather than resolving via the SDK
//     default chain, which hits IMDS).
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
	ConfigStatus string `json:"configstatus"`
	Version      string `json:"version"`
}

// getAgentRunningStatus returns the agent's running status ("running"/"stopped")
// as reported by the agent control script.
func getAgentRunningStatus(t *testing.T) string {
	t.Helper()
	out, err := common.RunCommand("sudo " + agentCtl + " -a status")
	require.NoError(t, err, "Failed to get agent status")
	var s agentStatus
	require.NoError(t, json.Unmarshal([]byte(out), &s), "Failed to parse agent status: %s", out)
	return s.Status
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

// TestAppSignalsNoCredentialsStartup verifies the agent starts up in onPrem mode
// using a credentials file, even when IMDS is unreachable.
func TestAppSignalsNoCredentialsStartup(t *testing.T) {
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
printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\naws_session_token=%s\n' "$AKID" "$SAK" "$TOK" | sudo tee ` + onPremCredsFile + `>/dev/null`
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

	assert.Equal(t, "running", getAgentRunningStatus(t),
		"agent should start in onPrem mode with a credentials file even when IMDS is unreachable")
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

	agentLog := common.ReadAgentLogfile(common.AgentLogFile)
	assert.NotContains(t, agentLog, "has no WithTransportOptions",
		"provisioner extension should build an SDK client that supports custom root CAs")
	assert.NotContains(t, agentLog, "unable to add custom RootCAs",
		"provisioner extension should accept AWS_CA_BUNDLE custom root CAs")

	assert.Equal(t, "running", getAgentRunningStatus(t),
		"agent should start with a custom AWS_CA_BUNDLE set")
}
