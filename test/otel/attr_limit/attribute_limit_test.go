//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package attr_limit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Node discovery — cached lookup of the 3 AttrLimit node names.
// ---------------------------------------------------------------------------

var (
	attrLimitOnce     sync.Once
	attrLimitLowNode  string
	attrLimitMidNode  string
	attrLimitHighNode string
	attrLimitErr      error
)

func discoverAttrLimitNodes(t *testing.T) (low, mid, high string) {
	t.Helper()
	attrLimitOnce.Do(func() {
		ctx := context.Background()
		results, err := queryCache.GetWithFilter(ctx, "container_cpu_usage_seconds_total", map[string]string{
			"~@resource.k8s.pod.name": "attr-limit-nginx.*",
		})
		if err != nil {
			attrLimitErr = fmt.Errorf("querying attr-limit pods: %w", err)
			return
		}
		for _, r := range results {
			r := r
			podName := r.Labels.Resource["k8s.pod.name"]
			nodeName := r.Labels.Resource["k8s.node.name"]
			if strings.Contains(podName, "attr-limit-nginx-low") && attrLimitLowNode == "" {
				attrLimitLowNode = nodeName
			}
			if strings.Contains(podName, "attr-limit-nginx-mid") && attrLimitMidNode == "" {
				attrLimitMidNode = nodeName
			}
			if strings.Contains(podName, "attr-limit-nginx-high") && attrLimitHighNode == "" {
				attrLimitHighNode = nodeName
			}
		}
		if attrLimitLowNode == "" || attrLimitMidNode == "" || attrLimitHighNode == "" {
			attrLimitErr = fmt.Errorf("could not discover all AttrLimit nodes: low=%q mid=%q high=%q (got %d results)",
				attrLimitLowNode, attrLimitMidNode, attrLimitHighNode, len(results))
		}
	})
	if attrLimitErr != nil {
		t.Fatalf("AttrLimit node discovery failed: %v", attrLimitErr)
	}
	return attrLimitLowNode, attrLimitMidNode, attrLimitHighNode
}

// ---------------------------------------------------------------------------
// TestPhase1NFDRemoval
// ---------------------------------------------------------------------------

func TestPhase1NFDRemoval(t *testing.T) {
	t.Parallel()
	for _, metricName := range daemonsetMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				for key := range r.Labels.Resource {
					if strings.Contains(key, "feature.node.kubernetes.io") {
						t.Errorf("%s has NFD label that should have been removed: %s (node: %s)",
							metricName, key, r.Labels.Resource["k8s.node.name"])
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPhase1RedundantKeyRemoval
// ---------------------------------------------------------------------------

var phase1RedundantKeys = []string{
	"k8s.node.label.topology.kubernetes.io/region",
	"k8s.node.label.topology.kubernetes.io/zone",
	"k8s.node.label.topology.ebs.csi.aws.com/zone",
	"k8s.node.label.node.kubernetes.io/instance-type",
	"k8s.node.label.kubernetes.io/hostname",
	"k8s.node.label.eks.amazonaws.com/nodegroup-image",
	"k8s.pod.label.pod-template-hash",
	"k8s.pod.label.controller-revision-hash",
}

func TestPhase1RedundantKeyRemoval(t *testing.T) {
	t.Parallel()
	for _, metricName := range daemonsetMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				for _, key := range phase1RedundantKeys {
					key := key
					if _, present := r.Labels.Resource[key]; present {
						t.Errorf("%s has Phase 1 redundant key that should have been removed: %s (node: %s)",
							metricName, key, r.Labels.Resource["k8s.node.name"])
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestProtectedAttributesSurvive
// ---------------------------------------------------------------------------

func TestProtectedAttributesSurvive(t *testing.T) {
	t.Parallel()
	nodeProtected := []string{"k8s.node.name", "cloud.region", "host.type"}

	for _, metricName := range daemonsetMetricNames() {
		metricName := metricName
		t.Run(metricName+"/node_attrs", func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				for _, key := range nodeProtected {
					key := key
					val, ok := r.Labels.Resource[key]
					require.True(t, ok,
						"%s missing protected attr @resource.%s (node: %s)",
						metricName, key, r.Labels.Resource["k8s.node.name"])
					require.True(t, val != "",
						"%s has empty protected attr @resource.%s", metricName, key)
				}
			}
		})
	}

	podProtected := []string{"k8s.pod.name", "k8s.namespace.name"}

	for _, metricName := range podMetricNames() {
		metricName := metricName
		t.Run(metricName+"/pod_attrs", func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
					continue
				}
				for _, key := range podProtected {
					key := key
					val, ok := r.Labels.Resource[key]
					require.True(t, ok,
						"%s missing protected attr @resource.%s (pod: %s)",
						metricName, key, r.Labels.Resource["k8s.pod.name"])
					require.True(t, val != "",
						"%s has empty protected attr @resource.%s", metricName, key)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAttributeCountLimit
// ---------------------------------------------------------------------------

const attributeLimit = 150

func TestAttributeCountLimit(t *testing.T) {
	t.Parallel()
	for _, metricName := range daemonsetMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				r := r
				total := len(r.Labels.Resource) +
					len(r.Labels.Datapoint) +
					len(r.Labels.Instrumentation) +
					len(r.Labels.AWS) +
					len(r.Labels.AWSCloudWatch)
				// maxExpected allows 20 fixed system/instrumentation attributes
				// beyond the 150-attr cap (e.g. __name__, cluster name, host.type,
				// instrumentation scope labels, and OTel-injected resource attrs).
				const maxExpected = attributeLimit + 20
				if total > maxExpected {
					t.Errorf("%s exceeds expected max: got %d, max %d (node: %s, pod: %s)",
						metricName, total, maxExpected,
						r.Labels.Resource["k8s.node.name"],
						r.Labels.Resource["k8s.pod.name"])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 2 tier-dropping sentinel keys.
// ---------------------------------------------------------------------------

var tier1SentinelKeys = []string{
	"k8s.node.label.helm.sh/chart",
	"k8s.node.label.release",
}

var tier2SentinelKeys = []string{
	"k8s.node.label.pod-template-generation",
}

var tier3VendorSentinelKeys = []string{
	"k8s.node.label.karpenter.sh/sentinel-known",
	"k8s.node.label.nvidia.com/sentinel-known",
}

var tier4EKSSentinelKeys = []string{
	"k8s.node.label.eks.amazonaws.com/capacityType",
	"k8s.node.label.eks.amazonaws.com/nodegroup",
}

var tier6CustomerNodeSentinelKeys = []string{
	"k8s.node.label.ci-test.example.com/sentinel-customer-a",
	"k8s.node.label.ci-test.example.com/sentinel-customer-b",
}

var tier7KnownPodSentinelKeys = []string{
	"k8s.pod.label.app.kubernetes.io/part-of",
}

var tier8CustomerPodSentinelKeys = []string{
	"k8s.pod.label.ci-test.example.com/sentinel-pod-customer",
}

// ---------------------------------------------------------------------------
// TestPhase2TierDroppingLow
// ---------------------------------------------------------------------------

func TestPhase2TierDroppingLow(t *testing.T) {
	t.Parallel()
	low, _, _ := discoverAttrLimitNodes(t)

	lowResults, err := queryCache.GetWithFilter(context.Background(), "container_cpu_usage_seconds_total", map[string]string{
		"@resource.k8s.node.name": low,
		"~@resource.k8s.pod.name": "attr-limit-nginx-low.*",
	})
	require.NoError(t, err, "querying container_cpu_usage_seconds_total for attr-limit-nginx-low")
	require.NotEmpty(t, lowResults, "no container_cpu_usage_seconds_total results for nginx pod on low node %s", low)

	for _, r := range lowResults {
		r := r
		for _, key := range tier1SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("low node %s: Tier 1 sentinel %s should have been dropped but is present", low, key)
			}
		}
		for _, key := range tier2SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("low node %s: Tier 2 sentinel %s should have been dropped but is present", low, key)
			}
		}
		for _, key := range tier6CustomerNodeSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; !present {
				t.Errorf("low node %s: Tier 6 sentinel %s should have survived but is absent", low, key)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TestPhase2TierDroppingMid
// ---------------------------------------------------------------------------

func TestPhase2TierDroppingMid(t *testing.T) {
	t.Parallel()
	_, mid, _ := discoverAttrLimitNodes(t)

	midResults, err := queryCache.GetWithFilter(context.Background(), "container_cpu_usage_seconds_total", map[string]string{
		"@resource.k8s.node.name": mid,
		"~@resource.k8s.pod.name": "attr-limit-nginx-mid.*",
	})
	require.NoError(t, err, "querying container_cpu_usage_seconds_total for attr-limit-nginx-mid")
	require.NotEmpty(t, midResults, "no container_cpu_usage_seconds_total results for nginx pod on mid node %s", mid)

	for _, r := range midResults {
		r := r
		for _, key := range tier1SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("mid node %s: Tier 1 sentinel %s should have been dropped but is present", mid, key)
			}
		}
		for _, key := range tier2SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("mid node %s: Tier 2 sentinel %s should have been dropped but is present", mid, key)
			}
		}
		for _, key := range tier3VendorSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("mid node %s: Tier 3 vendor sentinel %s should have been dropped but is present", mid, key)
			}
		}
		for _, key := range tier4EKSSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("mid node %s: Tier 4 EKS sentinel %s should have been dropped but is present", mid, key)
			}
		}
		for _, key := range tier6CustomerNodeSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; !present {
				t.Errorf("mid node %s: Tier 6 sentinel %s should have survived but is absent", mid, key)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TestPhase2TierDroppingHigh
// ---------------------------------------------------------------------------

func TestPhase2TierDroppingHigh(t *testing.T) {
	t.Parallel()
	_, _, high := discoverAttrLimitNodes(t)

	highResults, err := queryCache.GetWithFilter(context.Background(), "container_cpu_usage_seconds_total", map[string]string{
		"@resource.k8s.node.name": high,
		"~@resource.k8s.pod.name": "attr-limit-nginx-high.*",
	})
	require.NoError(t, err, "querying container_cpu_usage_seconds_total for attr-limit-nginx-high")
	require.NotEmpty(t, highResults, "no container_cpu_usage_seconds_total results for nginx pod on high node %s", high)

	for _, r := range highResults {
		r := r
		for _, key := range tier1SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 1 sentinel %s should have been dropped but is present", high, key)
			}
		}
		for _, key := range tier2SentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 2 sentinel %s should have been dropped but is present", high, key)
			}
		}
		for _, key := range tier3VendorSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 3 vendor sentinel %s should have been dropped but is present", high, key)
			}
		}
		for _, key := range tier4EKSSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 4 EKS sentinel %s should have been dropped but is present", high, key)
			}
		}
		for _, key := range tier6CustomerNodeSentinelKeys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 6 customer node sentinel %s should have been dropped but is present", high, key)
			}
		}
	}

	tier78Keys := append(tier7KnownPodSentinelKeys, tier8CustomerPodSentinelKeys...)
	for _, r := range highResults {
		r := r
		for _, key := range tier78Keys {
			key := key
			if _, present := r.Labels.Resource[key]; present {
				t.Errorf("high node %s: Tier 7/8 pod sentinel %s should have been dropped but is present (pod: %s)",
					high, key, r.Labels.Resource["k8s.pod.name"])
			}
		}
	}
}
