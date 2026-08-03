//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package efa

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestEFAPrerequisites — verify the EFA test infrastructure exists.
// ---------------------------------------------------------------------------

func TestEFAPrerequisites(t *testing.T) {
	t.Parallel()
	gt := getGroundTruth(t)
	t.Run("efa-test-allocated_exists", func(t *testing.T) {
		t.Parallel()
		_, found := gt.lookupPod("efa-test-allocated", "default")
		require.True(t, found,
			"efa-test-allocated pod not found — bare Pod may have been deleted")
	})
}

// TestEFAInstrumentationSource validates that all EFA metrics
// have @instrumentation.@name == awsefareceiver scope.
func TestEFAInstrumentationSource(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, scopeEFA, name, "%s instrumentation name", metricName)
			}
		})
	}
}

// TestEFAPodName validates that at least some EFA results have pod name.
func TestEFAPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var correlated []otelmetrics.MetricResult
			for _, r := range results {
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					correlated = append(correlated, r)
				}
			}
			require.True(t, len(correlated) > 0,
				"No %s results have @resource.k8s.pod.name — is efaburn running?", metricName)
			for _, r := range correlated {
				_, hasNS := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, hasNS, "%s correlated result missing @resource.k8s.namespace.name", metricName)
				_, hasNode := r.Labels.Resource["k8s.node.name"]
				require.True(t, hasNode, "%s correlated result missing @resource.k8s.node.name", metricName)
			}
		})
	}
}

// TestEFADevice validates that all EFA results have a non-empty aws.efa.device at datapoint level.
func TestEFADevice(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				device, ok := r.Labels.Datapoint["aws.efa.device"]
				require.True(t, ok, "%s missing datapoint aws.efa.device", metricName)
				require.True(t, device != "", "%s has empty datapoint aws.efa.device", metricName)
			}
		})
	}
}

// TestEFAEniId validates that all EFA results have aws.efa.eni.id at datapoint level.
func TestEFAEniId(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				eniID, ok := r.Labels.Datapoint["aws.efa.eni.id"]
				require.True(t, ok, "%s missing datapoint aws.efa.eni.id", metricName)
				require.True(t, eniID != "", "%s has empty datapoint aws.efa.eni.id", metricName)
			}
		})
	}
}

// TestEFAMultiNodeCoverage validates that efa_rx_bytes is reported from at least 1 node.
func TestEFAMultiNodeCoverage(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available (no EFA nodes?)")

	nodes := make(map[string]struct{})
	for _, r := range results {
		if node, ok := r.Labels.Resource["k8s.node.name"]; ok {
			nodes[node] = struct{}{}
		}
	}
	require.True(t, len(nodes) >= 1, "No EFA nodes found in results")
	if len(nodes) < 2 {
		t.Logf("WARNING: Only %d EFA node(s) reporting metrics, expected 2", len(nodes))
	}
}

// TestEFAActiveNodePodLabels validates that at least one result has
// "efaburn" or "efa-test-allocated" in the pod name.
func TestEFAActiveNodePodLabels(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available (no EFA nodes?)")

	hasEfaburn := false
	hasAllocated := false
	for _, r := range results {
		if pod, ok := r.Labels.Resource["k8s.pod.name"]; ok {
			if strings.Contains(pod, "efaburn") {
				hasEfaburn = true
			}
			if strings.Contains(pod, "efa-test-allocated") {
				hasAllocated = true
			}
		}
	}
	require.True(t, hasEfaburn || hasAllocated,
		"Expected efaburn or efa-test-allocated in pod names")
}

// TestEFADeviceAttributes validates EFA-specific device attributes at datapoint level.
func TestEFADeviceAttributes(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				efaDevice, hasDevice := r.Labels.Datapoint["aws.efa.device"]
				require.True(t, hasDevice, "%s missing datapoint aws.efa.device", metricName)
				require.True(t, efaDevice != "", "%s has empty datapoint aws.efa.device", metricName)

				eniID, hasENI := r.Labels.Datapoint["aws.efa.eni.id"]
				require.True(t, hasENI, "%s missing datapoint aws.efa.eni.id", metricName)
				require.True(t, strings.HasPrefix(eniID, "eni-"),
					"%s aws.efa.eni.id should start with 'eni-', got '%s'", metricName, eniID)

				_, hasPort := r.Labels.Datapoint["aws.efa.port"]
				require.True(t, hasPort, "%s missing datapoint aws.efa.port", metricName)

				if pod := getAnyValue(r, "k8s.pod.name"); pod != "" {
					cn := getAnyValue(r, "k8s.container.name")
					require.True(t, cn != "",
						"%s correlated result missing k8s.container.name (pod: %s)",
						metricName, pod)
				}
			}
		})
	}
}

// TestEFAEfaburnContainerName validates that efaburn pod results have
// k8s.container.name=efaburn.
func TestEFAEfaburnContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var efaburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efaburn") {
					efaburn = append(efaburn, r)
				}
			}
			require.True(t, len(efaburn) > 0, "No %s results from efaburn pods", metricName)
			for _, r := range efaburn {
				cn, ok := r.Labels.Resource["k8s.container.name"]
				require.True(t, ok, "%s efaburn result missing k8s.container.name", metricName)
				require.Equal(t, "efaburn", cn, "%s efaburn container name", metricName)
			}
		})
	}
}

// TestEFAAllocatedPodContainerName validates that efa-test-allocated pod results
// have k8s.container.name=efa-test.
func TestEFAAllocatedPodContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var allocated []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efa-test-allocated") {
					allocated = append(allocated, r)
				}
			}
			require.NotEmpty(t, allocated, "No %s results from efa-test-allocated pod", metricName)
			for _, r := range allocated {
				cn, ok := r.Labels.Resource["k8s.container.name"]
				require.True(t, ok, "%s efa-test-allocated result missing k8s.container.name", metricName)
				require.Equal(t, "efa-test", cn, "%s efa-test-allocated container name", metricName)
			}
		})
	}
}

// TestEFAUncorrelatedNoContainerName validates that uncorrelated EFA results
// (no k8s.pod.name) do NOT have k8s.container.name.
func TestEFAUncorrelatedNoContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				if _, hasPod := r.Labels.Resource["k8s.pod.name"]; !hasPod {
					_, has := r.Labels.Resource["k8s.container.name"]
					require.False(t, has,
						"%s uncorrelated EFA result should not have k8s.container.name but got: %s",
						metricName, r.Labels.Resource["k8s.container.name"])
				}
			}
		})
	}
}

// TestEFANoServiceInstanceId validates that EFA metrics do NOT have
// service.instance.id (EFA uses awsefareceiver, not Prometheus scrape).
func TestEFANoServiceInstanceId(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				_, has := r.Labels.Resource["service.instance.id"]
				require.False(t, has,
					"%s EFA metric should not have service.instance.id but got: %s (node: %s)",
					metricName, r.Labels.Resource["service.instance.id"], r.Labels.Resource["k8s.node.name"])
			}
		})
	}
}

// TestEFAEfaburnWorkloadLabels validates that efaburn pod results have
// correct workload labels.
func TestEFAEfaburnWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var efaburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efaburn") {
					efaburn = append(efaburn, r)
				}
			}
			require.True(t, len(efaburn) > 0, "No %s results from efaburn pods", metricName)
			for _, r := range efaburn {
				require.Equal(t, "efaburn", r.Labels.Resource["k8s.workload.name"], "%s efaburn pod k8s.workload.name", metricName)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "%s efaburn pod k8s.workload.type", metricName)
				require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s efaburn pod k8s.namespace.name", metricName)
				require.Equal(t, "efaburn", r.Labels.Resource["k8s.deployment.name"], "%s efaburn pod k8s.deployment.name", metricName)
			}
		})
	}
}

// TestEFABarePodNoWorkloadLabels validates that efa-test-allocated (bare Pod)
// has namespace=default but does NOT have k8s.workload.name.
func TestEFABarePodNoWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var allocated []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efa-test-allocated") {
					allocated = append(allocated, r)
				}
			}
			require.NotEmpty(t, allocated, "No %s results from efa-test-allocated pod", metricName)
			for _, r := range allocated {
				require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s efa-test-allocated pod k8s.namespace.name", metricName)
				_, hasName := r.Labels.Resource["k8s.workload.name"]
				require.False(t, hasName,
					"%s efa-test-allocated (bare Pod) should not have k8s.workload.name but got: %s",
					metricName, r.Labels.Resource["k8s.workload.name"])
			}
		})
	}
}

// TestEFANoPromotedDatapointKeys validates that k8s.pod.name, k8s.namespace.name,
// and k8s.container.name are absent from the Datapoint scope after transform/efa_promote.
func TestEFANoPromotedDatapointKeys(t *testing.T) {
	t.Parallel()
	promotedKeys := []string{"k8s.pod.name", "k8s.namespace.name", "k8s.container.name"}
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			for _, r := range results {
				for _, key := range promotedKeys {
					_, has := r.Labels.Datapoint[key]
					require.False(t, has,
						"%s should not have datapoint '%s' after promotion but got: %s",
						metricName, key, r.Labels.Datapoint[key])
				}
			}
		})
	}
}

// TestEFAEfaburnPodColor validates that efaburn pod results have podColorLabel=teal.
func TestEFAEfaburnPodColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no EFA nodes?)", metricName)
			var efaburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efaburn") {
					efaburn = append(efaburn, r)
				}
			}
			require.True(t, len(efaburn) > 0, "No %s results from efaburn pods", metricName)
			for _, r := range efaburn {
				require.Equal(t, "teal", r.Labels.Resource[podColorLabel], "%s efaburn expected pod-color=teal", metricName)
			}
		})
	}
}

// TestEFANodeGroupCoverage validates that EFA metrics are present on EFA-enabled nodes.
func TestEFANodeGroupCoverage(t *testing.T) {
	t.Parallel()
	escaped := escapePromQL(cfg.ClusterName)
	t.Run("c5n.9xlarge", func(t *testing.T) {
		t.Parallel()
		promql := fmt.Sprintf(
			`efa_rx_bytes{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
			escaped, "c5n.9xlarge")
		results, err := client.Query(context.Background(), promql)
		require.NoError(t, err, "querying efa_rx_bytes on c5n.9xlarge")
		require.True(t, len(results) > 0,
			"EFA metrics missing from c5n.9xlarge nodes — awsefareceiver not collecting?")
	})
}
