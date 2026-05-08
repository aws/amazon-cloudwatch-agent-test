//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Scope version test validates that the @instrumentation.@version label
// on DCGM metrics matches the dcgm-exporter pod image tag. Catches drift
// between the helm chart's pinned dcgm-exporter version and what the agent
// reports as the scope version.
//
// Ports the monorepo TestScopeNameAndVersionPerSource/dcgm subtest.

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDCGMScopeVersion validates @instrumentation.@version on DCGM metrics
// matches the dcgm-exporter pod's image tag.
func TestDCGMScopeVersion(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	// dcgm-exporter is labeled app.kubernetes.io/component=dcgm-exporter in
	// amazon-cloudwatch namespace (matches the helm chart's DaemonSet).
	dcgmVersion := imageTagFromPod(t, gt, "amazon-cloudwatch",
		"app.kubernetes.io/component", "dcgm-exporter")
	require.NotEmpty(t, dcgmVersion,
		"could not determine dcgm-exporter image tag from K8s API — is the pod running?")
	t.Logf("expected dcgm-exporter scope version = %s", dcgmVersion)

	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")

	version, ok := results[0].Labels.Instrumentation["@version"]
	require.True(t, ok, "DCGM_FI_DEV_GPU_UTIL missing @instrumentation.@version")
	require.Equal(t, dcgmVersion, version,
		"DCGM scope version should match dcgm-exporter pod image tag (got %q, want %q)",
		version, dcgmVersion)
}
