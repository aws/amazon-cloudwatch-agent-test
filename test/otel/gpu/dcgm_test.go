//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

var dcgmMetricNamesList = metricNames(dcgmMetrics)

// TestDCGMInstrumentationSource validates that all DCGM metrics
// have @instrumentation.@name == "github.com/NVIDIA/dcgm-exporter".
func TestDCGMInstrumentationSource(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				name, ok := r.Labels.Instrumentation["@name"]
				require.True(t, ok, "%s missing @instrumentation.@name", metricName)
				require.Equal(t, "github.com/NVIDIA/dcgm-exporter", name, "%s instrumentation name", metricName)
			}
		})
	}
}

// TestDCGMPodName validates that at least some DCGM results have pod name.
func TestDCGMPodName(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			var correlated []otelmetrics.MetricResult
			for _, r := range results {
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					correlated = append(correlated, r)
				}
			}
			require.True(t, len(correlated) > 0,
				"No %s results have @resource.k8s.pod.name — is multi-gpu-burn running?", metricName)
			for _, r := range correlated {
				_, hasNS := r.Labels.Resource["k8s.namespace.name"]
				require.True(t, hasNS, "%s correlated result missing @resource.k8s.namespace.name", metricName)
				_, hasNode := r.Labels.Resource["k8s.node.name"]
				require.True(t, hasNode, "%s correlated result missing @resource.k8s.node.name", metricName)
			}
		})
	}
}

// TestDCGMNamespace validates that correlated DCGM results have namespace.
func TestDCGMNamespace(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				if _, ok := r.Labels.Resource["k8s.pod.name"]; ok {
					_, hasNS := r.Labels.Resource["k8s.namespace.name"]
					require.True(t, hasNS, "%s correlated result missing namespace", metricName)
				}
			}
		})
	}
}

// TestDCGMMultiNodeCoverage validates that DCGM_FI_DEV_GPU_UTIL is reported
// from at least 2 GPU nodes (g4dn.xlarge + g4dn.12xlarge).
func TestDCGMMultiNodeCoverage(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")

	nodes := make(map[string]struct{})
	for _, r := range results {
		if node, ok := r.Labels.Resource["host.name"]; ok {
			nodes[node] = struct{}{}
		}
	}
	require.True(t, len(nodes) >= 2,
		"Expected DCGM metrics from at least 2 GPU nodes, got %d", len(nodes))
}

// TestDCGMActiveNodeNonzero validates that at least one GPU node has
// non-zero utilization (multi-gpu-burn should be running on g4dn.12xlarge).
func TestDCGMActiveNodeNonzero(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "DCGM_FI_DEV_GPU_UTIL")
	require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL")
	require.NotEmpty(t, results, "DCGM_FI_DEV_GPU_UTIL not available (no GPU nodes?)")

	hasNonzero := false
	for _, r := range results {
		if r.Value > 0 {
			hasNonzero = true
			break
		}
	}
	require.True(t, hasNonzero, "All GPU utilization values are zero — is multi-gpu-burn running?")
}

// TestDCGMDeviceAttributes validates that all DCGM results have the expected
// device datapoint attributes: gpu, UUID, device, modelName, DCGM_FI_DRIVER_VERSION.
func TestDCGMDeviceAttributes(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				gpu, hasGPU := r.Labels.Datapoint["gpu"]
				require.True(t, hasGPU, "%s missing datapoint gpu", metricName)
				require.True(t, gpu != "", "%s has empty datapoint gpu", metricName)

				uuid, hasUUID := r.Labels.Datapoint["UUID"]
				require.True(t, hasUUID, "%s missing datapoint UUID", metricName)
				require.True(t, uuid != "", "%s has empty datapoint UUID", metricName)
				require.True(t, strings.HasPrefix(uuid, "GPU-"),
					"%s UUID should start with 'GPU-', got '%s'", metricName, uuid)

				device, hasDevice := r.Labels.Datapoint["device"]
				require.True(t, hasDevice, "%s missing datapoint device", metricName)
				require.True(t, device != "", "%s has empty datapoint device", metricName)
				require.True(t, strings.HasPrefix(device, "nvidia"),
					"%s device should start with 'nvidia', got '%s'", metricName, device)

				model, hasModel := r.Labels.Datapoint["modelName"]
				require.True(t, hasModel, "%s missing datapoint modelName", metricName)
				require.True(t, model != "", "%s has empty datapoint modelName", metricName)

				driverVer, hasDriver := r.Labels.Datapoint["DCGM_FI_DRIVER_VERSION"]
				require.True(t, hasDriver, "%s missing datapoint DCGM_FI_DRIVER_VERSION", metricName)
				require.True(t, driverVer != "", "%s has empty datapoint DCGM_FI_DRIVER_VERSION", metricName)
			}
		})
	}
}

// TestDCGMNoHwAttributes validates that NO DCGM results have any hw.* attributes.
func TestDCGMNoHwAttributes(t *testing.T) {
	t.Parallel()
	hwAttrs := []string{"hw.type", "hw.vendor", "hw.model", "hw.name", "hw.id"}
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				for _, attr := range hwAttrs {
					_, has := r.Labels.Resource[attr]
					require.False(t, has,
						"%s should not have @resource.%s but got: %s",
						metricName, attr, r.Labels.Resource[attr])
				}
			}
		})
	}
}

// TestDCGMExpectedLabels validates that metrics with expected_labels
// have those labels present in the Datapoint scope.
func TestDCGMExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range dcgmMetrics {
		md := md
		if len(md.ExpectedLabels) == 0 {
			continue
		}
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", md.Name)
			for _, r := range results {
				for _, label := range md.ExpectedLabels {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// TestDCGMGpuburnContainerName validates that multi-gpu-burn pod results have
// k8s.container.name=gpu-burn for all DCGM metrics.
// In this ephemeral cluster, multi-gpu-burn on g4dn.12xlarge is the active workload.
func TestDCGMGpuburnContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			var gpuburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "multi-gpu-burn") {
					gpuburn = append(gpuburn, r)
				}
			}
			require.True(t, len(gpuburn) > 0, "No %s results from multi-gpu-burn pods", metricName)
			for _, r := range gpuburn {
				cn, ok := r.Labels.Resource["k8s.container.name"]
				require.True(t, ok, "%s multi-gpu-burn result missing k8s.container.name", metricName)
				require.Equal(t, "gpu-burn", cn, "%s multi-gpu-burn container name", metricName)
			}
		})
	}
}

// TestDCGMIdleNodeNoContainerName validates that uncorrelated DCGM results
// (from idle g4dn.xlarge) do NOT have k8s.container.name.
func TestDCGMIdleNodeNoContainerName(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			idle := filterByHostType(results, "g4dn.xlarge")
			require.NotEmpty(t, idle, "No %s results from idle g4dn.xlarge node", metricName)
			for _, r := range idle {
				_, has := r.Labels.Resource["k8s.container.name"]
				require.False(t, has,
					"%s idle GPU node should not have k8s.container.name but got: %s",
					metricName, r.Labels.Resource["k8s.container.name"])
			}
		})
	}
}

// TestDCGMGpuburnWorkloadLabels validates that multi-gpu-burn pod results have
// correct workload labels for all DCGM metrics.
func TestDCGMGpuburnWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			var gpuburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "multi-gpu-burn") {
					gpuburn = append(gpuburn, r)
				}
			}
			require.True(t, len(gpuburn) > 0, "No %s results from multi-gpu-burn pods", metricName)
			for _, r := range gpuburn {
				require.Equal(t, "multi-gpu-burn", r.Labels.Resource["k8s.workload.name"], "%s multi-gpu-burn pod k8s.workload.name", metricName)
				require.Equal(t, "Deployment", r.Labels.Resource["k8s.workload.type"], "%s multi-gpu-burn pod k8s.workload.type", metricName)
				require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s multi-gpu-burn pod k8s.namespace.name", metricName)
				require.Equal(t, "multi-gpu-burn", r.Labels.Resource["k8s.deployment.name"], "%s multi-gpu-burn pod k8s.deployment.name", metricName)
			}
		})
	}
}

// TestDCGMIdleNodeNoWorkloadLabels validates that DCGM results from the idle
// g4dn.xlarge node do NOT have k8s.workload.name.
func TestDCGMIdleNodeNoWorkloadLabels(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			idle := filterByHostType(results, "g4dn.xlarge")
			require.NotEmpty(t, idle, "No results from idle g4dn.xlarge node for %s", metricName)
			for _, r := range idle {
				_, hasName := r.Labels.Resource["k8s.workload.name"]
				require.False(t, hasName,
					"%s idle GPU node has unexpected k8s.workload.name: %s",
					metricName, r.Labels.Resource["k8s.workload.name"])
			}
		})
	}
}

// TestDCGMGpuburnPodColor validates that multi-gpu-burn pod results have
// podColorLabel=magenta for all DCGM metrics.
func TestDCGMGpuburnPodColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			var gpuburn []otelmetrics.MetricResult
			for _, r := range results {
				if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "multi-gpu-burn") {
					gpuburn = append(gpuburn, r)
				}
			}
			require.True(t, len(gpuburn) > 0, "No %s results from multi-gpu-burn pods", metricName)
			for _, r := range gpuburn {
				require.Equal(t, "magenta", r.Labels.Resource[podColorLabel], "%s multi-gpu-burn pod expected pod-color=magenta", metricName)
			}
		})
	}
}

// TestDCGMNoStaleDatapointKeys validates that Hostname and pci_bus_id are
// cleaned up from datapoint scope.
func TestDCGMNoStaleDatapointKeys(t *testing.T) {
	t.Parallel()
	staleKeys := []string{"Hostname", "pci_bus_id"}
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, r := range results {
				for _, key := range staleKeys {
					_, has := r.Labels.Datapoint[key]
					require.False(t, has,
						"%s should not have datapoint '%s' after cleanup but got: %s",
						metricName, key, r.Labels.Datapoint[key])
				}
			}
		})
	}
}

// TestDCGMHasRawPromotedKeys validates that raw Prometheus names (pod, namespace, container)
// are preserved in datapoint scope alongside semconv names in resource scope.
func TestDCGMHasRawPromotedKeys(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "container"}
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			for _, key := range rawKeys {
				found := false
				for _, r := range results {
					if val, ok := r.Labels.Datapoint[key]; ok && val != "" {
						found = true
						break
					}
				}
				require.True(t, found,
					"%s should have %s at datapoint scope on at least some results",
					metricName, key)
			}
		})
	}
}

// TestDCGMIdleNodeNoPodColor validates that DCGM results from the idle
// g4dn.xlarge node do NOT have the pod color label.
func TestDCGMIdleNodeNoPodColor(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			idle := filterByHostType(results, "g4dn.xlarge")
			require.NotEmpty(t, idle, "No results from idle g4dn.xlarge node for %s", metricName)
			for _, r := range idle {
				_, has := r.Labels.Resource[podColorLabel]
				require.False(t, has,
					"%s idle GPU node should not have %s", metricName, podColorLabel)
			}
		})
	}
}

// TestDCGMNodeGroupCoverage validates that DCGM metrics are present
// on all GPU node groups.
func TestDCGMNodeGroupCoverage(t *testing.T) {
	t.Parallel()
	escaped := otelmetrics.EscapePromQLValue(cfg.ClusterName)
	for _, ng := range gpuInstanceTypes {
		ng := ng
		t.Run(ng.Description+"/"+ng.InstanceType, func(t *testing.T) {
			t.Parallel()
			promql := fmt.Sprintf(
				`DCGM_FI_DEV_GPU_UTIL{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
				escaped, ng.InstanceType)
			results, err := client.Query(context.Background(), promql)
			require.NoError(t, err, "querying DCGM_FI_DEV_GPU_UTIL on %s", ng.Description)
			require.True(t, len(results) > 0,
				"DCGM missing from %s nodes (%s) — dcgm-exporter not running?",
				ng.Description, ng.InstanceType)
		})
	}
}
