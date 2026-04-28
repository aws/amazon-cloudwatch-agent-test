//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Deduplication tests validate that metrics are not duplicated by the batch
// processor. A known bug causes the batch processor to split resource-identical
// metrics when one batch has @schema_url set and the other doesn't, producing
// duplicate series in CloudWatch.

package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNodeExporterNoDuplicateSeries queries node_cpu_seconds_total for a single
// (node, cpu, mode) combination and asserts exactly 1 series is returned.
func TestNodeExporterNoDuplicateSeries(t *testing.T) {
	ctx := context.Background()
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cfg.ClusterName)

	promql := fmt.Sprintf(
		`node_cpu_seconds_total{"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`,
		escaped, clusterHostTypes[0],
	)
	results, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	r := results[0]
	pinNode := r.Labels.Resource["k8s.node.name"]
	pinCPU := r.Labels.Datapoint["cpu"]
	pinMode := r.Labels.Datapoint["mode"]
	require.True(t, pinNode != "" && pinCPU != "" && pinMode != "",
		"Could not find node_cpu_seconds_total result with node+cpu+mode")

	promql = fmt.Sprintf(
		`node_cpu_seconds_total{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s",cpu="%s",mode="%s"}`,
		escaped, pinNode, pinCPU, pinMode,
	)
	pinned, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying pinned node_cpu_seconds_total")
	require.NotEmpty(t, pinned, "pinned node_cpu_seconds_total returned 0 results")
	require.Equal(t, 1, len(pinned),
		"Expected exactly 1 series for node_cpu_seconds_total{node=%s,cpu=%s,mode=%s}, got %d — possible @schema_url duplication",
		pinNode, pinCPU, pinMode, len(pinned))
}

// TestCadvisorNoDuplicateSeries queries container_memory_working_set_bytes for a
// single (node, pod, container) combination and asserts exactly 1 series.
func TestCadvisorNoDuplicateSeries(t *testing.T) {
	ctx := context.Background()
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cfg.ClusterName)

	promql := fmt.Sprintf(
		`container_memory_working_set_bytes{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"nginx-test.*"}`,
		escaped,
	)
	results, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying container_memory_working_set_bytes for nginx-test")
	require.NotEmpty(t, results, "container_memory_working_set_bytes not available for nginx-test pods")

	var pinNode, pinPod, pinContainer string
	for _, r := range results {
		node := r.Labels.Resource["k8s.node.name"]
		pod := r.Labels.Resource["k8s.pod.name"]
		cn := r.Labels.Resource["k8s.container.name"]
		if node != "" && pod != "" && cn != "" {
			pinNode = node
			pinPod = pod
			pinContainer = cn
			break
		}
	}
	require.True(t, pinPod != "", "Could not find container_memory_working_set_bytes result from nginx-test pod")

	promql = fmt.Sprintf(
		`container_memory_working_set_bytes{"@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s","@resource.k8s.pod.name"="%s","@resource.k8s.container.name"="%s"}`,
		escaped, pinNode, pinPod, pinContainer,
	)
	pinned, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying pinned container_memory_working_set_bytes")
	require.NotEmpty(t, pinned, "pinned container_memory_working_set_bytes returned 0 results")
	require.Equal(t, 1, len(pinned),
		"Expected exactly 1 series for container_memory_working_set_bytes{node=%s,pod=%s,container=%s}, got %d — possible @schema_url duplication",
		pinNode, pinPod, pinContainer, len(pinned))
}

// TestNodeExporterNoSchemaUrlLabel validates that node_exporter metrics do NOT
// have @resource.@schema_url as a label.
func TestNodeExporterNoSchemaUrlLabel(t *testing.T) {
	for _, metricName := range nodeExporterMetricNames {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				_, has := r.Labels.Resource["@schema_url"]
				require.True(t, !has,
					"%s has @resource.@schema_url which causes series duplication (node: %s)",
					metricName, r.Labels.Resource["k8s.node.name"])
			}
		})
	}
}

// TestKubeletstatsNoDuplicateSeries queries k8s.node.cpu.utilization for a single
// node and asserts exactly 1 series is returned.
func TestKubeletstatsNoDuplicateSeries(t *testing.T) {
	ctx := context.Background()
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cfg.ClusterName)

	results, err := queryCache.Get(ctx, "k8s.node.cpu.utilization")
	require.NoError(t, err, "querying k8s.node.cpu.utilization")
	require.NotEmpty(t, results, "k8s.node.cpu.utilization not available")

	r := results[0]
	pinNode := r.Labels.Resource["k8s.node.name"]
	require.True(t, pinNode != "", "Could not find k8s.node.cpu.utilization result with k8s.node.name")

	promql := fmt.Sprintf(
		`{"__name__"="k8s.node.cpu.utilization","@resource.k8s.cluster.name"="%s","@resource.k8s.node.name"="%s"}`,
		escaped, pinNode,
	)
	pinned, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying pinned k8s.node.cpu.utilization")
	require.NotEmpty(t, pinned, "pinned k8s.node.cpu.utilization returned 0 results")
	require.Equal(t, 1, len(pinned),
		"Expected exactly 1 series for k8s.node.cpu.utilization{node=%s}, got %d — possible @schema_url duplication",
		pinNode, len(pinned))
}
