//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package karpenter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKarpenterMetricsExist verifies that each expected Karpenter metric is present.
func TestKarpenterMetricsExist(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (is Karpenter installed?)", metricName)
		})
	}
}

// TestKarpenterInstrumentation verifies instrumentation source for all Karpenter metrics.
func TestKarpenterInstrumentation(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopeKarpenter, name, "%s instrumentation name", metricName)
			}
		})
	}
}

// TestKarpenterClusterName verifies the cluster name resource attribute is set correctly.
func TestKarpenterClusterName(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				cluster, ok := r.Labels.Resource["k8s.cluster.name"]
				require.True(t, ok, "%s missing @resource.k8s.cluster.name", metricName)
				require.Equal(t, cfg.ClusterName, cluster, "%s cluster name mismatch", metricName)
			}
		})
	}
}

// TestKarpenterExpectedLabels verifies that expected datapoint labels are present.
func TestKarpenterExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range karpenterMetrics {
		md := md
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				r := r
				for _, label := range md.ExpectedLabels {
					label := label
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// TestKarpenterNamespace verifies that Karpenter metrics have a namespace resource attribute.
func TestKarpenterNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				ns, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing @resource.k8s.namespace.name", metricName)
				require.NotEmpty(t, ns, "%s empty k8s.namespace.name", metricName)
			}
		})
	}
}
