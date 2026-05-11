//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Deduplication tests validate that metrics are not duplicated by the batch
// processor. A known bug causes the batch processor to split resource-identical
// metrics when one batch has @schema_url set and the other doesn't, producing
// duplicate series in CloudWatch.

package gpu

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestDCGMNoDuplicateSeries queries DCGM_FI_DEV_GPU_UTIL for a single
// (node, GPU UUID) combination and asserts exactly 1 series is returned.
func TestDCGMNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	escaped := otelmetrics.EscapePromQLValue(cfg.ClusterName)

	// Get DCGM results from GPU nodes.
	results, err := queryCache.Get(ctx, "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")

	// Pin on node + GPU UUID to isolate a single series.
	var pinNode, pinUUID string
	for _, r := range results {
		if n, ok := r.Labels.Resource["k8s.node.name"]; ok && n != "" {
			if u, ok := r.Labels.Datapoint["UUID"]; ok && u != "" {
				pinNode = n
				pinUUID = u
				break
			}
		}
	}
	require.True(t, pinNode != "" && pinUUID != "",
		"Could not find DCGM_FI_DEV_GPU_UTIL result with node+UUID")

	promql := fmt.Sprintf(
		`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s",UUID="%s"}`,
		escaped, pinNode, pinUUID,
	)
	pinned, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying pinned DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, pinned, "pinned DCGM_FI_DEV_GPU_UTIL returned 0 results")
	require.Equal(t, 1, len(pinned),
		"Expected exactly 1 series for DCGM_FI_DEV_GPU_UTIL{node=%s,UUID=%s}, got %d — possible @schema_url duplication",
		pinNode, pinUUID, len(pinned))
}
