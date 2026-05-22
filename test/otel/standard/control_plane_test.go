//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestControlPlaneInstrumentation — instrumentation source for control plane metrics.
// ---------------------------------------------------------------------------

func TestControlPlaneInstrumentation(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopePrometheus, name, "%s instrumentation name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAPIServerComponentLabel — apiserver metrics must have k8s.component.name=apiserver.
// ---------------------------------------------------------------------------

func TestAPIServerComponentLabel(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (apiserver not scraped?)", metricName)
			for _, r := range results {
				comp, ok := r.Labels.Resource["k8s.component.name"]
				require.True(t, ok, "%s missing @resource.k8s.component.name", metricName)
				require.Equal(t, "apiserver", comp, "%s k8s.component.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneExpectedLabels — expected datapoint labels present.
// ---------------------------------------------------------------------------

func TestControlPlaneExpectedLabels(t *testing.T) {
	for _, md := range controlPlaneMetrics {
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		for _, label := range md.ExpectedLabels {
			t.Run(md.Name+"/"+label, func(t *testing.T) {
				results, err := queryCache.Get(context.Background(), md.Name)
				require.NoError(t, err, "querying %s", md.Name)
				require.NotEmpty(t, results, "%s not available (control plane not scraped?)", md.Name)
				for _, r := range results {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneClusterIdentity — @resource.k8s.cluster.name present.
// ---------------------------------------------------------------------------

func TestControlPlaneClusterIdentity(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				require.Equal(t, cfg.ClusterName, r.Labels.Resource["k8s.cluster.name"],
					"%s cluster name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneCloudDetection — cloud.* from resourcedetection.
// ---------------------------------------------------------------------------

func TestControlPlaneCloudDetection(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName+"/provider", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				require.Equal(t, "aws", r.Labels.Resource["cloud.provider"], "%s cloud.provider", metricName)
			}
		})
		t.Run(metricName+"/region", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				region, ok := r.Labels.Resource["cloud.region"]
				require.True(t, ok, "%s missing @resource.cloud.region", metricName)
				require.True(t, region != "", "%s has empty cloud.region", metricName)
			}
		})
		t.Run(metricName+"/account_id", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				acctID, ok := r.Labels.Resource["cloud.account.id"]
				require.True(t, ok, "%s missing @resource.cloud.account.id", metricName)
				require.Equal(t, 12, len(acctID), "%s cloud.account.id length", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneInstrumentationVersion — @instrumentation.@version present.
// ---------------------------------------------------------------------------

func TestControlPlaneInstrumentationVersion(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				version, ok := r.Labels.Instrumentation["@version"]
				require.True(t, ok, "%s missing @instrumentation.@version", metricName)
				require.True(t, version != "", "%s has empty @instrumentation.@version", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneNoNodeLabels — must NOT have k8s.node.name or k8s.node.uid.
// ---------------------------------------------------------------------------

func TestControlPlaneNoNodeLabels(t *testing.T) {
	for _, metricName := range metricNames(controlPlaneMetrics) {
		t.Run(metricName+"/no_node_name", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				_, hasNode := r.Labels.Resource["k8s.node.name"]
				require.True(t, !hasNode,
					"%s control plane metric should not have k8s.node.name but got: %s",
					metricName, r.Labels.Resource["k8s.node.name"])
			}
		})
		t.Run(metricName+"/no_node_uid", func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (control plane not scraped?)", metricName)
			for _, r := range results {
				_, hasUID := r.Labels.Resource["k8s.node.uid"]
				require.True(t, !hasUID,
					"%s control plane metric should not have k8s.node.uid but got: %s",
					metricName, r.Labels.Resource["k8s.node.uid"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestControlPlaneMultipleInstances — confirm per-node scraping, not load balancer.
// ---------------------------------------------------------------------------

func TestControlPlaneMultipleInstances(t *testing.T) {
	metricName := "apiserver_current_inflight_requests"
	results, err := queryCache.Get(context.Background(), metricName)
	require.NoError(t, err, "querying %s", metricName)
	require.NotEmpty(t, results, "%s not available (apiserver not scraped?)", metricName)

	instances := make(map[string]struct{})
	for _, r := range results {
		if v, ok := r.Labels.Resource["service.instance.id"]; ok && v != "" {
			instances[v] = struct{}{}
		}
	}

	keys := make([]string, 0, len(instances))
	for k := range instances {
		keys = append(keys, k)
	}
	t.Logf("found %d results from %d distinct API server instance(s): %v",
		len(results), len(instances), keys)

	require.True(t, len(instances) >= 2,
		"expected metrics from >= 2 distinct API server instances, got %d: %v",
		len(instances), keys)
}
