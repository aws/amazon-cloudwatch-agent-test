//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestCrossTelemetryHostID — for a given k8s.node.name, host.id in logs
// matches host.id in metrics.
// ---------------------------------------------------------------------------

func TestCrossTelemetryHostID(t *testing.T) {
	// Get host.id from metrics (node_exporter is node-scoped, always has host.id).
	metricResults, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err)
	require.NotEmpty(t, metricResults)

	// Build node→host.id map from metrics.
	metricHostIDs := make(map[string]string) // k8s.node.name → host.id
	for _, r := range metricResults {
		node := r.Labels.Resource["k8s.node.name"]
		hostID := r.Labels.Resource["host.id"]
		if node != "" && hostID != "" {
			metricHostIDs[node] = hostID
		}
	}
	require.NotEmpty(t, metricHostIDs, "no metrics with both k8s.node.name and host.id")

	// Get app logs and verify host.id matches for the same node.
	logResults, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, logResults)

	matched := 0
	for _, lr := range logResults {
		node := lr.Resource["k8s.node.name"]
		logHostID := lr.Resource["host.id"]
		if node == "" || logHostID == "" {
			continue
		}
		if metricHostID, ok := metricHostIDs[node]; ok {
			matched++
			require.Equal(t, metricHostID, logHostID,
				"host.id mismatch for node %s: metrics=%s logs=%s",
				node, metricHostID, logHostID)
		}
	}
	require.True(t, matched > 0,
		"no app logs matched a node from metrics — cannot verify cross-telemetry consistency")
}

// ---------------------------------------------------------------------------
// TestCrossTelemetryCloudResourceID — cloud.resource_id format matches
// between logs and metrics.
// ---------------------------------------------------------------------------

func TestCrossTelemetryCloudResourceID(t *testing.T) {
	expectedPrefix := "arn:aws:eks:"
	expectedSuffix := ":cluster/" + cfg.ClusterName

	// Check app logs.
	logResults, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, logResults)

	for _, r := range logResults {
		arn := r.Resource["cloud.resource_id"]
		require.True(t, strings.HasPrefix(arn, expectedPrefix),
			"app log cloud.resource_id should start with %q, got %q", expectedPrefix, arn)
		require.True(t, strings.HasSuffix(arn, expectedSuffix),
			"app log cloud.resource_id should end with %q, got %q", expectedSuffix, arn)
	}
}
