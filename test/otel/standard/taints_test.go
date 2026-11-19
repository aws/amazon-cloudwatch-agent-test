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

// Test taints applied to the first node:
//   ci-test.example.com/dedicated=gpu:NoSchedule
//   ci-test.example.com/empty=:NoExecute

const (
	taintKeyDedicated = "k8s.node.taint.ci-test.example.com/dedicated"
	taintKeyEmpty     = "k8s.node.taint.ci-test.example.com/empty"
	taintPrefix       = "k8s.node.taint.ci-test.example.com/"
)

func TestTaintsAppearOnDaemonSetMetrics(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	var found bool
	for _, r := range results {
		if r.Labels.Resource[taintKeyDedicated] == "gpu" {
			found = true
			break
		}
	}
	require.True(t, found, "expected at least one node_cpu_seconds_total series with %s=gpu", taintKeyDedicated)
}

func TestTaintsEmptyValueSkipped(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	for _, r := range results {
		if r.Labels.Resource[taintKeyDedicated] != "gpu" {
			continue
		}
		_, hasEmpty := r.Labels.Resource[taintKeyEmpty]
		require.False(t, hasEmpty, "empty-value taint %s should NOT be present", taintKeyEmpty)
		return
	}
	t.Fatal("no tainted series found to verify empty-value taint absence")
}

func TestTaintsAbsentOnUntaintedNodes(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	var untainted int
	for _, r := range results {
		hasTaint := false
		for k := range r.Labels.Resource {
			if strings.HasPrefix(k, taintPrefix) {
				hasTaint = true
				break
			}
		}
		if !hasTaint {
			untainted++
		}
	}
	require.Greater(t, untainted, 0, "expected at least one series from untainted node with no %s* attrs", taintPrefix)
}

func TestTaintsAppearOnClusterScraperMetrics(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "kube_node_info")
	require.NoError(t, err, "querying kube_node_info")
	require.NotEmpty(t, results, "kube_node_info not available")

	var found bool
	for _, r := range results {
		if r.Labels.Resource[taintKeyDedicated] == "gpu" {
			found = true
			break
		}
	}
	require.True(t, found, "expected at least one kube_node_info series with %s=gpu (cluster-scraper)", taintKeyDedicated)
}

func TestTaintsMultipleOnSameNode(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	for _, r := range results {
		if r.Labels.Resource[taintKeyDedicated] != "gpu" {
			continue
		}
		// The tainted node also has ci-test.example.com/team=ml
		val, hasTeam := r.Labels.Resource["k8s.node.taint.ci-test.example.com/team"]
		require.True(t, hasTeam, "tainted node should have both test taints present")
		require.Equal(t, "ml", val)
		return
	}
	t.Fatal("no tainted series found to verify multiple taints")
}
