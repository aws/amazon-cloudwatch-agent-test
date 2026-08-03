//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Scope version test validates that the @instrumentation.@version on EFA
// metrics matches the cloudwatch-agent pod image tag (EFA is an agent-
// internal receiver: awsefareceiver). When the image tag is "latest" or a
// git SHA (>=32 chars), the agent reports its own semver build version
// instead — in that case we fall back to asserting @version is non-empty.
//
// Ports the monorepo TestScopeNameAndVersionPerSource/efa subtest.

package efa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEFAScopeVersion validates @instrumentation.@version on EFA metrics
// matches the cloudwatch-agent pod's image tag (or is non-empty when the
// agent is running a "latest" dev image).
func TestEFAScopeVersion(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	agentVersion := imageTagFromPod(t, gt, "amazon-cloudwatch",
		"app.kubernetes.io/name", "cloudwatch-agent")
	require.NotEmpty(t, agentVersion,
		"could not determine cloudwatch-agent image tag from K8s API")
	t.Logf("cloudwatch-agent image tag = %s", agentVersion)

	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available")

	version, ok := results[0].Labels.Instrumentation["@version"]
	require.True(t, ok, "efa_rx_bytes missing @instrumentation.@version")
	require.NotEmpty(t, version, "efa_rx_bytes has empty @instrumentation.@version")

	if agentVersion == "latest" || len(agentVersion) >= 32 {
		t.Logf("agent image tag %q is not a release tag (dev/SHA build); scope version = %s",
			agentVersion, version)
		return
	}

	require.Equal(t, agentVersion, version,
		"EFA scope version should match cloudwatch-agent pod image tag (got %q, want %q)",
		version, agentVersion)
}
