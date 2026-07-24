//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package keda

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
// TestKEDAMetricsExist — verify each expected metric is present in
// CloudWatch.
// ---------------------------------------------------------------------------

func TestKEDAMetricsExist(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			results, err := queryCache.Get(ctx, metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (is KEDA installed?)", metricName)
		})
	}
}

// ===========================================================================
// Instrumentation Scope Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKEDAInstrumentation — verify instrumentation scope name for all
// KEDA metrics.
// ---------------------------------------------------------------------------

func TestKEDAInstrumentation(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
				require.Equal(t, scopeKEDA, name, "%s instrumentation name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKEDAInstrumentationConsistent — verify all data points for a
// metric report the same instrumentation scope (no mixed sources).
// ---------------------------------------------------------------------------

func TestKEDAInstrumentationConsistent(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
// TestKEDAExpectedLabels — verify expected datapoint labels are present
// on metrics that declare them.
// ---------------------------------------------------------------------------

func TestKEDAExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range kedaMetrics {
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
// TestKEDAClusterIdentity — k8s.cluster.name must match the configured
// cluster.
// ---------------------------------------------------------------------------

func TestKEDAClusterIdentity(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
// TestKEDANamespace — all KEDA metrics must originate from the
// keda namespace.
// ---------------------------------------------------------------------------

func TestKEDANamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				ns, ok := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, ok, "%s missing @resource.k8s.namespace.name", metricName)
				require.Equal(t, "keda", ns, "%s k8s.namespace.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKEDADeploymentName — k8s.deployment.name must be "keda-operator".
// ---------------------------------------------------------------------------

func TestKEDADeploymentName(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				deploy, ok := r.Labels.Resource["k8s.deployment.name"]
				require.True(t, ok, "%s missing @resource.k8s.deployment.name", metricName)
				require.Equal(t, "keda-operator", deploy, "%s k8s.deployment.name", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKEDAPodName — k8s.pod.name must start with "keda-operator-".
// ---------------------------------------------------------------------------

func TestKEDAPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName, ok := r.Labels.Resource["k8s.pod.name"]
				require.True(t, ok, "%s missing @resource.k8s.pod.name", metricName)
				require.True(t, strings.HasPrefix(podName, "keda-operator-"),
					"%s k8s.pod.name should start with 'keda-operator-', got %q", metricName, podName)
			}
		})
	}
}

// ===========================================================================
// Cloud Resource Attribute Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestKEDACloudProvider — cloud.provider must be "aws".
// ---------------------------------------------------------------------------

func TestKEDACloudProvider(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
// TestKEDACloudPlatform — cloud.platform must be "aws_eks".
// ---------------------------------------------------------------------------

func TestKEDACloudPlatform(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
// TestKEDACloudRegion — cloud.region must match the configured region.
// ---------------------------------------------------------------------------

func TestKEDACloudRegion(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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
// TestKEDACloudAccountID — cloud.account.id must match the test account.
// ---------------------------------------------------------------------------

func TestKEDACloudAccountID(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				acctID, ok := r.Labels.Resource["cloud.account.id"]
				if ok {
					require.Equal(t, cfg.AccountID, acctID, "%s cloud.account.id", metricName)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKEDACloudResourceID — cloud.resource_id must be a valid EKS
// cluster ARN.
// ---------------------------------------------------------------------------

func TestKEDACloudResourceID(t *testing.T) {
	t.Parallel()
	expectedARNPrefix := fmt.Sprintf("arn:aws:eks:%s:", cfg.Region)
	expectedARNSuffix := fmt.Sprintf(":cluster/%s", cfg.ClusterName)

	for _, metricName := range kedaMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				arn, ok := r.Labels.Resource["cloud.resource_id"]
				if ok {
					require.True(t, strings.HasPrefix(arn, expectedARNPrefix),
						"%s cloud.resource_id should start with %q, got %q", metricName, expectedARNPrefix, arn)
					require.True(t, strings.HasSuffix(arn, expectedARNSuffix),
						"%s cloud.resource_id should end with %q, got %q", metricName, expectedARNSuffix, arn)
				}
			}
		})
	}
}
