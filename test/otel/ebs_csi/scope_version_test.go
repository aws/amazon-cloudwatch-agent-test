//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Scope version test validates that the @instrumentation.@version on EBS CSI
// metrics matches the cloudwatch-agent pod image tag (EBS CSI is scraped by
// the agent's prometheus receiver, so the scope version is the agent's).
// If the image tag is "latest" (dev builds), the agent reports its own
// build version — in that case we assert @version is non-empty.
//
// Ports the monorepo TestScopeNameAndVersionPerSource/ebs_csi subtest.

package ebs_csi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEBSCSIScopeVersion validates @instrumentation.@version on EBS CSI
// metrics matches the cloudwatch-agent pod's image tag (or is non-empty
// when the agent is running a "latest" dev image).
func TestEBSCSIScopeVersion(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	agentVersion := imageTagFromPod(t, gt, "amazon-cloudwatch",
		"app.kubernetes.io/name", "cloudwatch-agent")
	require.NotEmpty(t, agentVersion,
		"could not determine cloudwatch-agent image tag from K8s API")
	t.Logf("cloudwatch-agent image tag = %s", agentVersion)

	results, err := queryCache.Get(context.Background(), "aws_ebs_csi_read_ops_total")
	require.NoError(t, err, "querying aws_ebs_csi_read_ops_total")
	require.NotEmpty(t, results, "aws_ebs_csi_read_ops_total not available")

	version, ok := results[0].Labels.Instrumentation["@version"]
	require.True(t, ok, "aws_ebs_csi_read_ops_total missing @instrumentation.@version")
	require.NotEmpty(t, version, "aws_ebs_csi_read_ops_total has empty @instrumentation.@version")

	if agentVersion == "latest" {
		t.Logf("agent image tag is 'latest' (dev build); scope version = %s", version)
		return
	}

	require.Equal(t, agentVersion, version,
		"EBS CSI scope version should match cloudwatch-agent pod image tag (got %q, want %q)",
		version, agentVersion)
}
