//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Metric resolution tests validate that each metric source is scraped at the
// expected 30-second interval.

package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const expectedResolution = 30 * time.Second

// The EKS test also carries the EC2 host.type of its node group and filters on it. GKE nodes have none,
// so the query is scoped to the cluster only.
var resolutionTestMetrics = []struct {
	name   string
	source string
}{
	{"node_load1", "node_exporter"},
	{"container_cpu_usage_seconds_total", "cadvisor"},
}

// TestMetricResolution validates that metrics are scraped at ~30s intervals by
// querying a 5-minute range and checking sample counts.
func TestMetricResolution(t *testing.T) {
	t.Parallel()
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	step := expectedResolution

	expectedSamples := int(5*time.Minute/expectedResolution) + 1 // 11 for 30s
	minSamples := expectedSamples / 2

	for _, tm := range resolutionTestMetrics {
		tm := tm
		t.Run(tm.source+"/"+tm.name, func(t *testing.T) {
			t.Parallel()
			escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cfg.ClusterName)
			promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s"}`, tm.name, escaped)

			results, err := client.QueryRange(context.Background(), promql, start, end, step)
			require.NoError(t, err, "range querying %s", tm.name)
			require.True(t, len(results) > 0, "No %s range results", tm.name)

			var bestSeries *otelmetrics.RangeResult
			for i := range results {
				if bestSeries == nil || len(results[i].Timestamps) > len(bestSeries.Timestamps) {
					bestSeries = &results[i]
				}
			}

			t.Logf("%s: %d samples in 5-minute window (expected ~%d for %v resolution)",
				tm.name, len(bestSeries.Timestamps), expectedSamples, expectedResolution)

			require.True(t, len(bestSeries.Timestamps) >= minSamples,
				"%s: got %d samples, expected at least %d for %v resolution",
				tm.name, len(bestSeries.Timestamps), minSamples, expectedResolution)
		})
	}
}
