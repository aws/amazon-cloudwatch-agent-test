//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Resolution test validates that EFA metrics are scraped at the expected
// 30-second interval. Ports the monorepo TestMetricResolution/efa subtest.

package efa

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	expectedEFAResolution     = 30 * time.Second
	efaResolutionInstanceType = "c5n.9xlarge"
)

// TestEFAResolution validates efa_rx_bytes is scraped at ~30s intervals on
// the c5n.9xlarge EFA nodes.
func TestEFAResolution(t *testing.T) {
	t.Parallel()
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	step := expectedEFAResolution

	expectedSamples := int(5*time.Minute/expectedEFAResolution) + 1 // 11 for 30s
	minSamples := expectedSamples / 2

	escaped := escapePromQL(cfg.ClusterName)
	promql := fmt.Sprintf(
		`efa_rx_bytes{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
		escaped, efaResolutionInstanceType)

	results, err := client.QueryRange(context.Background(), promql, start, end, step)
	require.NoError(t, err, "range querying efa_rx_bytes")
	require.True(t, len(results) > 0,
		"No efa_rx_bytes range results from %s", efaResolutionInstanceType)

	var bestSeries *otelmetrics.RangeResult
	for i := range results {
		if bestSeries == nil || len(results[i].Timestamps) > len(bestSeries.Timestamps) {
			bestSeries = &results[i]
		}
	}

	t.Logf("efa_rx_bytes: %d samples in 5-minute window (expected ~%d for %v resolution)",
		len(bestSeries.Timestamps), expectedSamples, expectedEFAResolution)

	require.True(t, len(bestSeries.Timestamps) >= minSamples,
		"efa_rx_bytes: got %d samples, expected at least %d for %v resolution",
		len(bestSeries.Timestamps), minSamples, expectedEFAResolution)
}
