//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestAppLogsExist — application logs are flowing to the expected log group.
// ---------------------------------------------------------------------------

func TestAppLogsExist(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err, "querying application logs")
	require.NotEmpty(t, results, "no application logs found in %s", appLogGroup())
}

// ---------------------------------------------------------------------------
// TestAppLogsResourceAttrs — application logs have all expected resource attrs.
// ---------------------------------------------------------------------------

func TestAppLogsResourceAttrs(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Core attrs (node + pod + container) must be on ALL logs.
	coreAttrs := otelmetrics.ContainerResourceAttrs
	for _, attr := range coreAttrs {
		t.Run(attr, func(t *testing.T) {
			for _, r := range results {
				v, ok := r.Resource[attr]
				require.True(t, ok, "app log missing resource.attributes.%s", attr)
				require.NotEmpty(t, v, "app log has empty resource.attributes.%s", attr)
			}
		})
	}

	// Workload attrs are present on most logs (pods with an owning workload),
	// but bare pods / static pods won't have them. Verify at least 50% do.
	workloadAttrs := []string{"k8s.workload.name", "k8s.workload.type", "service.name"}
	for _, attr := range workloadAttrs {
		t.Run(attr, func(t *testing.T) {
			count := 0
			for _, r := range results {
				if v, ok := r.Resource[attr]; ok && v != "" {
					count++
				}
			}
			require.True(t, count > len(results)/2,
				"app log resource.attributes.%s present on only %d/%d logs (expected majority)",
				attr, count, len(results))
		})
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsScopeAttrs — application logs have correct scope attributes.
// ---------------------------------------------------------------------------

func TestAppLogsScopeAttrs(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for key, want := range otelmetrics.AppLogScopeAttrs {
		t.Run(key, func(t *testing.T) {
			for _, r := range results {
				got, ok := r.Scope[key]
				require.True(t, ok, "app log missing scope.attributes.%s", key)
				require.Equal(t, want, got, "app log scope.attributes.%s", key)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsClusterIdentity — k8s.cluster.name matches test cluster.
// ---------------------------------------------------------------------------

func TestAppLogsClusterIdentity(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		require.Equal(t, cfg.ClusterName, r.Resource["k8s.cluster.name"],
			"app log k8s.cluster.name mismatch")
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsCloudProvider — cloud.provider == "aws".
// ---------------------------------------------------------------------------

func TestAppLogsCloudProvider(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		require.Equal(t, "aws", r.Resource["cloud.provider"], "app log cloud.provider")
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsHostType — host.type matches expected node group instance types.
// ---------------------------------------------------------------------------

func TestAppLogsHostType(t *testing.T) {
	validTypes := make(map[string]bool, len(clusterNodeGroups))
	for _, ng := range clusterNodeGroups {
		validTypes[ng.InstanceType] = true
	}

	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		ht := r.Resource["host.type"]
		require.True(t, validTypes[ht],
			"app log host.type=%q not in expected node groups %v", ht, validTypes)
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsBody — log body is non-empty.
// ---------------------------------------------------------------------------

func TestAppLogsBody(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		require.NotEmpty(t, r.Body, "app log has empty body")
	}
}

// ---------------------------------------------------------------------------
// TestAppLogsRecordAttrs — log record attributes (log.iostream, log.file.path).
// ---------------------------------------------------------------------------

func TestAppLogsRecordAttrs(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	t.Run("log.iostream", func(t *testing.T) {
		validStreams := map[string]bool{"stdout": true, "stderr": true}
		count := 0
		for _, r := range results {
			stream, ok := r.Attributes["log.iostream"]
			if !ok {
				continue
			}
			count++
			require.True(t, validStreams[stream],
				"app log attributes.log.iostream=%q, want stdout or stderr", stream)
		}
		require.True(t, count > len(results)/2,
			"log.iostream present on only %d/%d logs", count, len(results))
	})

	t.Run("log.file.path", func(t *testing.T) {
		for _, r := range results {
			path, ok := r.Attributes["log.file.path"]
			require.True(t, ok, "app log missing attributes.log.file.path")
			require.Contains(t, path, "/var/log/containers/",
				"app log log.file.path should be under /var/log/containers/")
		}
	})
}

// ---------------------------------------------------------------------------
// TestAppLogsWorkloadDerivation — workload name/type set correctly.
// ---------------------------------------------------------------------------

func TestAppLogsWorkloadDerivation(t *testing.T) {
	results, err := logQueryCache.Get(context.Background(), appLogGroup(), pipelineAppLogs)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	validTypes := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true,
		"Job": true, "CronJob": true, "ReplicaSet": true,
	}

	withWorkload := 0
	for _, r := range results {
		wType := r.Resource["k8s.workload.type"]
		if wType == "" {
			continue // bare pod / static pod — no workload owner
		}
		withWorkload++
		require.True(t, validTypes[wType],
			"app log k8s.workload.type=%q not a valid workload type", wType)

		// service.name should match workload name
		require.Equal(t, r.Resource["k8s.workload.name"], r.Resource["service.name"],
			"app log service.name should equal k8s.workload.name")
	}
	require.True(t, withWorkload > 0,
		"no app logs have k8s.workload.type — workload derivation may have failed entirely")
}
