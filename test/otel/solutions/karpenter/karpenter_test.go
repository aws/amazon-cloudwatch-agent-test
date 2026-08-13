//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package karpenter

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Metric Existence Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKarpenterMetricsExist — verify each expected metric is present in
// CloudWatch.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// TestKarpenterScalingMetricsPositive — verify event-driven scaling metrics
// have value >= 1, confirming Karpenter actually provisioned.
// ---------------------------------------------------------------------------

func TestKarpenterScalingMetricsPositive(t *testing.T) {
	t.Parallel()
	scalingMetrics := []string{
		"karpenter_nodeclaims_created_total",
		"karpenter_nodes_created_total",
	}
	for _, metricName := range scalingMetrics {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			var maxVal float64
			for _, r := range results {
				if r.Value > maxVal {
					maxVal = r.Value
				}
			}
			require.GreaterOrEqual(t, maxVal, float64(1),
				"%s should be >= 1 after provisioning, got %v", metricName, maxVal)
		})
	}
}

// ===========================================================================
// Instrumentation Scope Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKarpenterInstrumentation — verify instrumentation scope name for all
// Karpenter metrics.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// TestKarpenterInstrumentationConsistent — verify all data points for a
// metric report the same instrumentation scope (no mixed sources).
// ---------------------------------------------------------------------------

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

// ===========================================================================
// Datapoint Label Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKarpenterExpectedLabels — verify expected datapoint labels are present
// on metrics that declare them.
// ---------------------------------------------------------------------------

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

// ===========================================================================
// K8s Resource Attribute Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKarpenterClusterIdentity — k8s.cluster.name must match the configured
// cluster.
// ---------------------------------------------------------------------------

func TestKarpenterClusterIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				clusterName, ok := r.Labels.Resource["k8s.cluster.name"]
				require.True(t, ok, "%s missing @resource.k8s.cluster.name", metricName)
				require.Equal(t, cfg.ClusterName, clusterName, "%s k8s.cluster.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterNamespace — all Karpenter metrics must originate from the
// kube-system namespace.
// ---------------------------------------------------------------------------

func TestKarpenterNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				ns, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing @resource.k8s.namespace.name", metricName)
				require.Equal(t, "kube-system", ns, "%s k8s.namespace.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterDeploymentName — k8s.deployment.name must be "karpenter".
// ---------------------------------------------------------------------------

func TestKarpenterDeploymentName(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				deploy, ok := r.Labels.Resource["k8s.deployment.name"]
				require.True(t, ok, "%s missing @resource.k8s.deployment.name", metricName)
				require.Equal(t, "karpenter", deploy, "%s k8s.deployment.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterPodName — k8s.pod.name must start with "karpenter-".
// ---------------------------------------------------------------------------

func TestKarpenterPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName, ok := r.Labels.Resource["k8s.pod.name"]
				require.True(t, ok, "%s missing @resource.k8s.pod.name", metricName)
				require.True(t, strings.HasPrefix(podName, "karpenter-"),
					"%s k8s.pod.name should start with 'karpenter-', got %q", metricName, podName)
			}
		})
	}
}

// ===========================================================================
// Cloud Resource Attribute Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKarpenterCloudProvider — cloud.provider must be "aws".
// ---------------------------------------------------------------------------

func TestKarpenterCloudProvider(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				provider, ok := r.Labels.Resource["cloud.provider"]
				require.True(t, ok, "%s missing @resource.cloud.provider", metricName)
				require.Equal(t, "aws", provider, "%s cloud.provider", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterCloudPlatform — cloud.platform must be "aws_eks".
// ---------------------------------------------------------------------------

func TestKarpenterCloudPlatform(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				platform, ok := r.Labels.Resource["cloud.platform"]
				require.True(t, ok, "%s missing @resource.cloud.platform", metricName)
				require.Equal(t, "aws_eks", platform, "%s cloud.platform", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterCloudRegion — cloud.region must match the configured region.
// ---------------------------------------------------------------------------

func TestKarpenterCloudRegion(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				region, ok := r.Labels.Resource["cloud.region"]
				require.True(t, ok, "%s missing @resource.cloud.region", metricName)
				require.Equal(t, cfg.Region, region, "%s cloud.region", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterCloudAccountID — cloud.account.id must match the test account.
// ---------------------------------------------------------------------------

func TestKarpenterCloudAccountID(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				acctID, ok := r.Labels.Resource["cloud.account.id"]
				require.True(t, ok, "%s missing @resource.cloud.account.id", metricName)
				require.Equal(t, cfg.AccountID, acctID, "%s cloud.account.id", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKarpenterCloudResourceID — cloud.resource_id must be a valid EKS
// cluster ARN.
// ---------------------------------------------------------------------------

func TestKarpenterCloudResourceID(t *testing.T) {
	t.Parallel()
	expectedARNPrefix := fmt.Sprintf("arn:aws:eks:%s:", cfg.Region)
	expectedARNSuffix := fmt.Sprintf(":cluster/%s", cfg.ClusterName)

	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				arn, ok := r.Labels.Resource["cloud.resource_id"]
				require.True(t, ok, "%s missing @resource.cloud.resource_id", metricName)
				require.True(t, strings.HasPrefix(arn, expectedARNPrefix),
					"%s cloud.resource_id should start with %q, got %q", metricName, expectedARNPrefix, arn)
				require.True(t, strings.HasSuffix(arn, expectedARNSuffix),
					"%s cloud.resource_id should end with %q, got %q", metricName, expectedARNSuffix, arn)
			}
		})
	}
}
