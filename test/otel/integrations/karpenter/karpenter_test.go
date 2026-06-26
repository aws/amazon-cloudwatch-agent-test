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

// TestKarpenterInstrumentationConsistent verifies all data points for a metric
// report the same instrumentation scope (no mixed sources).
func TestKarpenterInstrumentationConsistent(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			names := make(map[string]struct{})
			for _, r := range results {
				if n, ok := r.Labels.Instrumentation["@name"]; ok {
					names[n] = struct{}{}
				}
			}
			require.Equal(t, 1, len(names), "%s has %d distinct instrumentation names", metricName, len(names))
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

// TestKarpenterResourceAttributes verifies K8s and cloud resource attributes are enriched
// correctly, including cluster name value validation.
func TestKarpenterResourceAttributes(t *testing.T) {
	t.Parallel()
	requiredAttrs := []string{
		"k8s.pod.name",
		"k8s.deployment.name",
		"k8s.namespace.name",
		"k8s.cluster.name",
		"cloud.provider",
		"cloud.region",
		"cloud.platform",
	}
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
				for _, attr := range requiredAttrs {
					v, ok := r.Labels.Resource[attr]
					require.True(t, ok, "%s missing @resource.%s", metricName, attr)
					require.NotEmpty(t, v, "%s empty @resource.%s", metricName, attr)
				}
				// Validate cluster name matches the expected value
				require.Equal(t, cfg.ClusterName, r.Labels.Resource["k8s.cluster.name"],
					"%s cluster name mismatch", metricName)
			}
		})
	}
}
