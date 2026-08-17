//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Resolution test validates that Neuron metrics are scraped at the expected
// 30-second interval. Ports the monorepo TestMetricResolution/neuron subtest.

package neuron

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	expectedNeuronResolution     = 30 * time.Second
	neuronResolutionInstanceType = "inf2.xlarge"
)

// TestNeuronResolution validates neuroncore_utilization_ratio is scraped at
// ~30s intervals on the workload inf2.xlarge node.
func TestNeuronResolution(t *testing.T) {
	t.Parallel()
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	step := expectedNeuronResolution

	expectedSamples := int(5*time.Minute/expectedNeuronResolution) + 1 // 11 for 30s
	minSamples := expectedSamples / 2

	escaped := escapePromQL(cfg.ClusterName)
	promql := fmt.Sprintf(
		`neuroncore_utilization_ratio{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
		escaped, neuronResolutionInstanceType)

	results, err := client.QueryRange(context.Background(), promql, start, end, step)
	require.NoError(t, err, "range querying neuroncore_utilization_ratio")
	require.True(t, len(results) > 0,
		"No neuroncore_utilization_ratio range results from %s", neuronResolutionInstanceType)

	// Pick the series with the most samples (best coverage).
	var bestSeries *otelmetrics.RangeResult
	for i := range results {
		if bestSeries == nil || len(results[i].Timestamps) > len(bestSeries.Timestamps) {
			bestSeries = &results[i]
		}
	}

	t.Logf("neuroncore_utilization_ratio: %d samples in 5-minute window (expected ~%d for %v resolution)",
		len(bestSeries.Timestamps), expectedSamples, expectedNeuronResolution)

	require.True(t, len(bestSeries.Timestamps) >= minSamples,
		"neuroncore_utilization_ratio: got %d samples, expected at least %d for %v resolution",
		len(bestSeries.Timestamps), minSamples, expectedNeuronResolution)
}
