//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Scope version test validates that the @instrumentation.@version label
// on Neuron metrics matches the neuron-monitor pod image tag. Catches drift
// between the helm chart's pinned neuron-monitor version and what the agent
// reports as the scope version.
//
// Ports the monorepo TestScopeNameAndVersionPerSource/neuron subtest.

package neuron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNeuronScopeVersion validates @instrumentation.@version on Neuron metrics
// matches the neuron-monitor pod's image tag.
func TestNeuronScopeVersion(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)

	// neuron-monitor is labeled app.kubernetes.io/component=neuron-monitor in
	// amazon-cloudwatch namespace (matches the helm chart's DaemonSet).
	neuronVersion := imageTagFromPod(t, gt, "amazon-cloudwatch",
		"app.kubernetes.io/component", "neuron-monitor")
	require.NotEmpty(t, neuronVersion,
		"could not determine neuron-monitor image tag from K8s API — is the pod running?")
	t.Logf("expected neuron-monitor scope version = %s", neuronVersion)

	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available (no Neuron nodes?)")

	version, ok := results[0].Labels.Instrumentation["@version"]
	require.True(t, ok, "neuroncore_utilization_ratio missing @instrumentation.@version")
	require.Equal(t, neuronVersion, version,
		"Neuron scope version should match neuron-monitor pod image tag (got %q, want %q)",
		version, neuronVersion)
}
