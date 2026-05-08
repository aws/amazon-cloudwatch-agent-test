//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Node-color tests validate that the custom node-color label applied by the
// Neuron cluster's terraform (red on inf2.xlarge workload+idle nodes, purple
// on inf2.24xlarge multi-device node) propagates to Neuron metrics.
//
// Ports the monorepo TestCustomNodeLabels/has_red subtest.

package neuron

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const nodeColorLabel = "k8s.node.label.ci-test.example.com/node-color"

// TestNeuronHasRedNodeColor validates Neuron metrics on inf2.xlarge nodes
// carry the node-color=red label applied by the workload/idle node groups.
func TestNeuronHasRedNodeColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			escaped := escapePromQL(cfg.ClusterName)
			promql := fmt.Sprintf(
				`%s{"@resource.k8s.cluster.name"="%s","@resource.%s"="red"}`,
				metricName, escaped, nodeColorLabel)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s with node-color=red filter", metricName)
			require.True(t, len(results) > 0,
				"%s: no results from inf2.xlarge nodes (expected node-color=red)", metricName)
		})
	}
}

// TestNeuronHasPurpleNodeColor validates Neuron metrics on the multi-device
// inf2.24xlarge node carry the node-color=purple label.
func TestNeuronHasPurpleNodeColor(t *testing.T) {
	t.Parallel()
	// Only core-level metrics are available on the multi-device burn pod
	// (runtime-level metrics require a running workload per runtime instance).
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			escaped := escapePromQL(cfg.ClusterName)
			promql := fmt.Sprintf(
				`%s{"@resource.k8s.cluster.name"="%s","@resource.%s"="purple"}`,
				md.Name, escaped, nodeColorLabel)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying %s with node-color=purple filter", md.Name)
			require.True(t, len(results) > 0,
				"%s: no results from inf2.24xlarge node (expected node-color=purple)", md.Name)
		})
	}
}
