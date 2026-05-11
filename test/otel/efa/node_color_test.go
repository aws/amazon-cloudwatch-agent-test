//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Node-color test validates the custom node-color=yellow label applied by
// the EFA cluster terraform propagates to EFA metrics. Ports the monorepo
// TestCustomNodeLabels/has_yellow subtest.

package efa

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const nodeColorLabel = "k8s.node.label.ci-test.example.com/node-color"

// TestEFAHasYellowNodeColor validates EFA metrics on c5n.9xlarge nodes
// carry the node-color=yellow label applied by the workload/idle node groups.
func TestEFAHasYellowNodeColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			escaped := escapePromQL(cfg.ClusterName)
			promql := fmt.Sprintf(
				`%s{"@resource.k8s.cluster.name"="%s","@resource.%s"="yellow"}`,
				metricName, escaped, nodeColorLabel)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s with node-color=yellow filter", metricName)
			require.True(t, len(results) > 0,
				"%s: no results from c5n.9xlarge nodes (expected node-color=yellow)", metricName)
		})
	}
}
