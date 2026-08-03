//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Deduplication tests validate that metrics are not duplicated by the batch
// processor. A known bug causes the batch processor to split resource-identical
// metrics when one batch has @schema_url set and the other doesn't, producing
// duplicate series in CloudWatch.

package efa

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEFANoDuplicateSeries queries efa_rx_bytes for a single (node, device)
// combination and asserts exactly 1 series is returned.
func TestEFANoDuplicateSeries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	escaped := escapePromQL(cfg.ClusterName)

	results, err := queryCache.Get(ctx, "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available (no EFA nodes?)")

	// Find a result that has both node name and device populated.
	// Device is at datapoint level.
	var pinNode, pinDevice string
	for _, r := range results {
		n := r.Labels.Resource["k8s.node.name"]
		d := r.Labels.Datapoint["aws.efa.device"]
		if n != "" && d != "" {
			pinNode = n
			pinDevice = d
			break
		}
	}
	require.True(t, pinNode != "" && pinDevice != "",
		"Could not find efa_rx_bytes result with node+aws.efa.device")

	promql := fmt.Sprintf(
		`efa_rx_bytes{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s","aws.efa.device"="%s"}`,
		escaped, pinNode, pinDevice,
	)
	pinned, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying pinned efa_rx_bytes")
	require.NotEmpty(t, pinned, "pinned efa_rx_bytes returned 0 results")
	require.Equal(t, 1, len(pinned),
		"Expected exactly 1 series for efa_rx_bytes{node=%s,aws.efa.device=%s}, got %d — possible @schema_url duplication",
		pinNode, pinDevice, len(pinned))
}
