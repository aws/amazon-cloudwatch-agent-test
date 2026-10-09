//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package kserve

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// pipelineResults returns the metric's series from the kserve-controlplane
// pipeline only.
func pipelineResults(t *testing.T, name string) []otelmetrics.MetricResult {
	t.Helper()
	results, err := queryCache.Get(context.Background(), name)
	require.NoError(t, err, "querying %s", name)
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if r.Labels.Instrumentation["cloudwatch.pipeline"] == pipelineKServe {
			out = append(out, r)
		}
	}
	require.NotEmpty(t, out, "%s not available from the %s pipeline (is KServe installed?)", name, pipelineKServe)
	return out
}

// ---------------------------------------------------------------------------
// TestKServeMetricsExist / TestKServeInstrumentation / TestKServeExpectedLabels
// ---------------------------------------------------------------------------

func TestKServeMetricsExist(t *testing.T) {
	t.Parallel()
	for _, name := range metricNames(kserveMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pipelineResults(t, name)
		})
	}
}

func TestKServeInstrumentation(t *testing.T) {
	t.Parallel()
	for _, name := range metricNames(kserveMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name) {
				require.Equal(t, scopeKServe, r.Labels.Instrumentation["@name"], "%s instrumentation name", name)
			}
		})
	}
}

func TestKServeExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range kserveMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, md.Name) {
				for _, label := range md.ExpectedLabels {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKServeAttribution — series describe the controller pod (found by label),
// not the cluster-scraper that scraped it: no AZ or host of the scraper's node.
// ---------------------------------------------------------------------------

func TestKServeAttribution(t *testing.T) {
	t.Parallel()
	for _, name := range metricNames(kserveMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name) {
				res := r.Labels.Resource
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				require.Equal(t, "kserve", res["k8s.namespace.name"], "k8s.namespace.name")
				require.Equal(t, "kserve-controller-manager", res["k8s.deployment.name"], "k8s.deployment.name")
				require.True(t, strings.HasPrefix(res["k8s.pod.name"], "kserve-controller-manager-"),
					"k8s.pod.name %q", res["k8s.pod.name"])
				requireCloudIdentity(t, name, res)
				for _, attr := range []string{"cloud.availability_zone", "host.name", "host.id"} {
					_, ok := res[attr]
					require.False(t, ok, "%s carries %s from the scraper's node", name, attr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKServeClientGoFamiliesDropped — workqueue_* and rest_client_* belong to
// the apiserver pipeline; the KServe pipeline must not publish them.
// ---------------------------------------------------------------------------

func TestKServeClientGoFamiliesDropped(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"workqueue_depth", "workqueue_adds_total", "rest_client_requests_total", "go_goroutines", "process_cpu_seconds_total"} {
		require.Empty(t, series(t, name, pipeline(pipelineKServe)), "%s should be dropped by the %s pipeline", name, pipelineKServe)
	}
}
