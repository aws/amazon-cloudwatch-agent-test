//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestEventsLogGroupExists — the events log group has records.
// ---------------------------------------------------------------------------

func TestEventsLogGroupExists(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err, "querying events log group")
	require.NotEmpty(t, results, "no event logs found in %s", eventsLogGroup())
}

// ---------------------------------------------------------------------------
// TestEventsResourceAttributes — every event has required resource attributes.
// ---------------------------------------------------------------------------

func TestEventsResourceAttributes(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	requiredAttrs := []string{
		"k8s.cluster.name",
		"k8s.object.kind",
		"k8s.object.name",
		"k8s.object.uid",
		"cloud.region",
		"cloud.account.id",
		"cloud.provider",
		"cloud.platform",
		"cloud.resource_id",
		"aws.log.group.names",
	}

	for _, attr := range requiredAttrs {
		t.Run(attr, func(t *testing.T) {
			for i, r := range results {
				v, ok := r.Resource[attr]
				require.True(t, ok, "event[%d] missing resource.attributes.%s (kind=%s, name=%s)",
					i, attr, r.Resource["k8s.object.kind"], r.Resource["k8s.object.name"])
				require.NotEmpty(t, v, "event[%d] resource.attributes.%s is empty", i, attr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestEventsClusterIdentity — cluster-level attributes are correct.
// ---------------------------------------------------------------------------

func TestEventsClusterIdentity(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	expectedPrefix := fmt.Sprintf("arn:aws:eks:%s:", cfg.Region)
	expectedSuffix := ":cluster/" + cfg.ClusterName

	for i, r := range results {
		require.Equal(t, cfg.ClusterName, r.Resource["k8s.cluster.name"],
			"event[%d] k8s.cluster.name mismatch", i)
		require.Equal(t, cfg.Region, r.Resource["cloud.region"],
			"event[%d] cloud.region mismatch", i)
		require.Equal(t, "aws", r.Resource["cloud.provider"],
			"event[%d] cloud.provider mismatch", i)
		require.Equal(t, "aws_eks", r.Resource["cloud.platform"],
			"event[%d] cloud.platform mismatch", i)

		rid := r.Resource["cloud.resource_id"]
		require.True(t, strings.HasPrefix(rid, expectedPrefix),
			"event[%d] cloud.resource_id %q should start with %q", i, rid, expectedPrefix)
		require.True(t, strings.HasSuffix(rid, expectedSuffix),
			"event[%d] cloud.resource_id %q should end with %q", i, rid, expectedSuffix)
	}
}

// ---------------------------------------------------------------------------
// TestEventsScopeAttributes — scope name and attributes are correct.
// ---------------------------------------------------------------------------

func TestEventsScopeAttributes(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	expectedScope := map[string]string{
		"cloudwatch.source":   "cloudwatch-agent",
		"cloudwatch.solution": "k8s-otel-container-insights",
		"cloudwatch.pipeline": "events",
	}

	for key, want := range expectedScope {
		t.Run(key, func(t *testing.T) {
			for i, r := range results {
				got, ok := r.Scope[key]
				require.True(t, ok, "event[%d] missing scope.attributes.%s", i, key)
				require.Equal(t, want, got, "event[%d] scope.attributes.%s", i, key)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestEventsLogAttributes — k8s.event.* attributes and body/severity present.
// ---------------------------------------------------------------------------

func TestEventsLogAttributes(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	requiredAttrs := []string{
		"k8s.event.reason",
		"k8s.event.uid",
		"k8s.event.name",
		"k8s.event.start_time",
	}

	for _, attr := range requiredAttrs {
		t.Run(attr, func(t *testing.T) {
			for i, r := range results {
				v, ok := r.Attributes[attr]
				require.True(t, ok, "event[%d] missing attributes.%s", i, attr)
				require.NotEmpty(t, v, "event[%d] attributes.%s is empty", i, attr)
			}
		})
	}

	t.Run("severityText", func(t *testing.T) {
		for i, r := range results {
			require.NotEmpty(t, r.SeverityText, "event[%d] missing severityText", i)
		}
	})

	t.Run("body", func(t *testing.T) {
		for i, r := range results {
			require.NotEmpty(t, r.Body, "event[%d] missing body", i)
		}
	})
}

// ---------------------------------------------------------------------------
// TestEventsPodEnrichment — Pod events are enriched with k8sattributes,
// nodemetadataenricher, and workload derivation.
// ---------------------------------------------------------------------------

func TestEventsPodEnrichment(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	var podEvents []int
	for i, r := range results {
		if r.Resource["k8s.object.kind"] == "Pod" {
			podEvents = append(podEvents, i)
		}
	}
	if len(podEvents) == 0 {
		t.Skip("no Pod events found — cluster may not have generated any yet")
	}

	// Receiver-sourced attributes — must always be present on Pod events.
	t.Run("receiver_attrs", func(t *testing.T) {
		for _, idx := range podEvents {
			r := results[idx]
			for _, key := range []string{"k8s.pod.name", "k8s.namespace.name"} {
				v := r.Resource[key]
				require.NotEmpty(t, v, "pod event[%d] missing receiver attribute %s (pod=%s)",
					idx, key, r.Resource["k8s.object.name"])
			}
		}
	})

	// Enrichment-sourced attributes — k8sattributes adds k8s.node.name;
	// nodemetadataenricher adds host.* and cloud.availability_zone. May be absent
	// if pod was deleted before enrichment ran. Verify majority of pod events
	// have these attributes (matches PR #697 logs convention).
	t.Run("enrichment_attrs", func(t *testing.T) {
		enrichmentAttrs := []string{
			"k8s.node.name",
			"host.id",
			"host.name",
			"host.type",
			"host.image.id",
			"cloud.availability_zone",
		}
		for _, key := range enrichmentAttrs {
			t.Run(key, func(t *testing.T) {
				count := 0
				for _, idx := range podEvents {
					if v, ok := results[idx].Resource[key]; ok && v != "" {
						count++
					}
				}
				require.True(t, count > len(podEvents)/2,
					"pod event resource.attributes.%s present on only %d/%d pod events (expected majority)",
					key, count, len(podEvents))
			})
		}
	})

	// At least one Pod event must have workload derivation.
	t.Run("workload_derivation", func(t *testing.T) {
		var hasWorkload bool
		for _, idx := range podEvents {
			r := results[idx]
			if r.Resource["k8s.workload.name"] != "" {
				hasWorkload = true
				require.NotEmpty(t, r.Resource["k8s.workload.type"],
					"pod event has k8s.workload.name but missing k8s.workload.type")
				break
			}
		}
		require.True(t, hasWorkload,
			"no Pod events had workload enrichment — expected at least one workload-owned pod event")
	})
}

// ---------------------------------------------------------------------------
// TestEventsKindDiversity — events for multiple K8s object kinds are present.
// ---------------------------------------------------------------------------

func TestEventsKindDiversity(t *testing.T) {
	results, err := eventsLogQueryCache.Get(context.Background(), eventsLogGroup(), pipelineEvents)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	kinds := make(map[string]bool)
	for _, r := range results {
		if k := r.Resource["k8s.object.kind"]; k != "" {
			kinds[k] = true
		}
	}
	t.Logf("event kinds seen: %v", kinds)
	require.True(t, len(kinds) >= 2,
		"expected at least 2 event kinds, got %d: %v", len(kinds), kinds)
}
