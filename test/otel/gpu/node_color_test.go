//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Node-color tests validate that the custom node-color label applied by the
// GPU cluster's terraform (green on g4dn.xlarge + g4dn.12xlarge) propagates
// to DaemonSet-enriched metrics.
//
// Ports the monorepo TestCustomNodeLabels/has_green subtest.

package gpu

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const nodeColorLabel = "k8s.node.label.ci-test.example.com/node-color"

// TestDCGMHasGreenNodeColor validates that DCGM metrics on GPU nodes carry
// the node-color=green label applied by the terraform node group.
func TestDCGMHasGreenNodeColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			escaped := otelmetrics.EscapePromQLValue(cfg.ClusterName)
			promql := fmt.Sprintf(
				`%s{"@resource.k8s.cluster.name"="%s","@resource.%s"="green"}`,
				metricName, escaped, nodeColorLabel)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s with node-color=green filter", metricName)
			require.True(t, len(results) > 0,
				"%s: no results from GPU nodes (expected node-color=green)", metricName)
		})
	}
}
