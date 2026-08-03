//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Resolution test validates that DCGM metrics are scraped at the expected
// 30-second interval. Ports the monorepo TestMetricResolution/dcgm subtest.

package gpu

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const expectedDCGMResolution = 30 * time.Second

// TestDCGMResolution validates DCGM_FI_DEV_GPU_UTIL is scraped at ~30s intervals
// by querying a 5-minute range and checking sample counts on the multi-GPU node.
func TestDCGMResolution(t *testing.T) {
	t.Parallel()
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	step := expectedDCGMResolution

	expectedSamples := int(5*time.Minute/expectedDCGMResolution) + 1 // 11 for 30s
	minSamples := expectedSamples / 2

	escaped := otelmetrics.EscapePromQLValue(cfg.ClusterName)
	promql := fmt.Sprintf(
		`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
		escaped, multiGpuInstanceType)

	results, err := client.QueryRange(context.Background(), promql, start, end, step)
	require.NoError(t, err, "range querying DCGM_FI_DEV_GPU_UTIL")
	require.True(t, len(results) > 0, "No DCGM_FI_DEV_GPU_UTIL range results from %s", multiGpuInstanceType)

	// Pick the series with the most samples (best coverage).
	var bestSeries *otelmetrics.RangeResult
	for i := range results {
		if bestSeries == nil || len(results[i].Timestamps) > len(bestSeries.Timestamps) {
			bestSeries = &results[i]
		}
	}

	t.Logf("DCGM_FI_DEV_GPU_UTIL: %d samples in 5-minute window (expected ~%d for %v resolution)",
		len(bestSeries.Timestamps), expectedSamples, expectedDCGMResolution)

	require.True(t, len(bestSeries.Timestamps) >= minSamples,
		"DCGM_FI_DEV_GPU_UTIL: got %d samples, expected at least %d for %v resolution",
		len(bestSeries.Timestamps), minSamples, expectedDCGMResolution)
}
