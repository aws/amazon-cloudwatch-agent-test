//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Series tests port test/otel/gpu dedup_test.go and resolution_test.go, per GPU node pool.

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

// TestDCGMNoDuplicateSeries validates one series per (node, GPU UUID) on each pool.
func TestDCGMNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	for _, pool := range []string{managedPool} {
		pool := pool
		t.Run(pool, func(t *testing.T) {
			t.Parallel()
			r := poolResults(t, "DCGM_FI_DEV_GPU_UTIL", pool)[0]
			node, uuid := r.Labels.Resource["k8s.node.name"], r.Labels.Datapoint["UUID"]
			require.NotEmpty(t, uuid, "result on %s without UUID", node)

			promql := fmt.Sprintf(`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s",UUID="%s"}`,
				otelmetrics.EscapePromQLValue(cfg.ClusterName), otelmetrics.EscapePromQLValue(node), otelmetrics.EscapePromQLValue(uuid))
			pinned, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying pinned DCGM_FI_DEV_GPU_UTIL")
			require.Len(t, pinned, 1, "expected exactly 1 series for DCGM_FI_DEV_GPU_UTIL{node=%s,UUID=%s}", node, uuid)
		})
	}
}

// TestDCGMResolution validates DCGM_FI_DEV_GPU_UTIL is scraped about every 30s on each pool.
func TestDCGMResolution(t *testing.T) {
	t.Parallel()
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	expectedSamples := int(5*time.Minute/expectedDCGMResolution) + 1
	minSamples := expectedSamples / 2

	for _, pool := range []string{managedPool} {
		pool := pool
		t.Run(pool, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"=~"aks-%s-.*"}`,
				otelmetrics.EscapePromQLValue(cfg.ClusterName), pool)
			results, err := client.QueryRange(context.Background(), promql, start, end, expectedDCGMResolution)
			require.NoError(t, err, "range querying DCGM_FI_DEV_GPU_UTIL on %s", pool)
			require.NotEmpty(t, results, "no DCGM_FI_DEV_GPU_UTIL range results from %s", pool)

			best := results[0]
			for _, r := range results[1:] {
				if len(r.Timestamps) > len(best.Timestamps) {
					best = r
				}
			}
			t.Logf("%s: %d samples in 5 minutes (expected ~%d)", pool, len(best.Timestamps), expectedSamples)
			require.GreaterOrEqual(t, len(best.Timestamps), minSamples, "%s: too few samples for %v resolution", pool, expectedDCGMResolution)
		})
	}
}
