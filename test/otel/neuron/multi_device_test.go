//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Multi-device Neuron tests — inf2.24xlarge has 6 Neuron devices × 2 cores = 12 cores.
// Validates device/core cardinality, integer indices, consecutive numbering,
// and pod-to-device/core correlation on the multi-device node.

package neuron

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// Topology of the multi-device Neuron node in this cluster.
const (
	multiDeviceInstanceType        = "inf2.24xlarge"
	expectedMultiNeuronDeviceCount = 6
	expectedMultiNeuronCoreCount   = 12
	expectedCoresPerNeuronDevice   = 2
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// escapePromQL escapes a string value for safe use inside PromQL label matches.
func escapePromQL(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// filterByHostType returns results matching the given host.type.
func filterByHostType(results []otelmetrics.MetricResult, hostType string) []otelmetrics.MetricResult {
	var filtered []otelmetrics.MetricResult
	for _, r := range results {
		r := r
		if r.Labels.Resource["host.type"] == hostType {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// uniqueDatapointValues returns unique values for a datapoint label key.
func uniqueDatapointValues(results []otelmetrics.MetricResult, key string) map[string]struct{} {
	vals := make(map[string]struct{})
	for _, r := range results {
		r := r
		if v, ok := r.Labels.Datapoint[key]; ok {
			vals[v] = struct{}{}
		}
	}
	return vals
}

// uniqueDatapointValuesList wraps the map-returning helper with a list variant.
func uniqueDatapointValuesList(results []otelmetrics.MetricResult, key string) []string {
	set := uniqueDatapointValues(results, key)
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}

// uniqueDatapointPairs collects distinct (a, b) pairs from two datapoint keys.
func uniqueDatapointPairs(results []otelmetrics.MetricResult, keyA, keyB string) [][2]string {
	set := make(map[[2]string]struct{})
	for _, r := range results {
		r := r
		a := r.Labels.Datapoint[keyA]
		b := r.Labels.Datapoint[keyB]
		if a == "" || b == "" {
			continue
		}
		set[[2]string{a, b}] = struct{}{}
	}
	out := make([][2]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

// isIntLike returns true if s is a non-negative integer string.
func isIntLike(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// ---------------------------------------------------------------------------
// Device/core cardinality tests
// ---------------------------------------------------------------------------

// TestMultiNeuronDeviceCount validates that the expected number of Neuron
// devices are visible on the multi-device node.
func TestMultiNeuronDeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0,
		"No neuroncore_utilization_ratio results from %s node", multiDeviceInstanceType)

	devices := uniqueDatapointValuesList(multi, "aws.neuron.device")
	require.Equal(t, expectedMultiNeuronDeviceCount, len(devices), fmt.Sprintf("Expected %d distinct aws.neuron.device on %s, got %d: %v",
		expectedMultiNeuronDeviceCount, multiDeviceInstanceType, len(devices), devices))
}

// TestMultiNeuronCoreCount validates the total (device, core) pair count.
func TestMultiNeuronCoreCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiDeviceInstanceType)

	pairs := uniqueDatapointPairs(multi, "aws.neuron.device", "aws.neuron.core")
	require.Equal(t, expectedMultiNeuronCoreCount, len(pairs), fmt.Sprintf("Expected %d (device, core) pairs on %s, got %d",
		expectedMultiNeuronCoreCount, multiDeviceInstanceType, len(pairs)))
}

// TestMultiNeuronCoresPerDevice validates each device exposes the expected cores.
func TestMultiNeuronCoresPerDevice(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiDeviceInstanceType)

	coresByDevice := make(map[string]map[string]struct{})
	for _, r := range multi {
		r := r
		dev := r.Labels.Datapoint["aws.neuron.device"]
		core := r.Labels.Datapoint["aws.neuron.core"]
		if dev == "" || core == "" {
			continue
		}
		if coresByDevice[dev] == nil {
			coresByDevice[dev] = make(map[string]struct{})
		}
		coresByDevice[dev][core] = struct{}{}
	}
	require.True(t, len(coresByDevice) > 0,
		"No devices with core metrics on %s", multiDeviceInstanceType)
	for dev, cores := range coresByDevice {
		require.Equal(t, expectedCoresPerNeuronDevice, len(cores), fmt.Sprintf("Neuron device %s: expected %d cores, got %d",
			dev, expectedCoresPerNeuronDevice, len(cores)))
	}
}

// TestMultiNeuronDeviceIndicesAreIntegers validates device and core are integers.
func TestMultiNeuronDeviceIndicesAreIntegers(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiDeviceInstanceType)

	for _, r := range multi {
		r := r
		dev := r.Labels.Datapoint["aws.neuron.device"]
		core := r.Labels.Datapoint["aws.neuron.core"]
		require.True(t, isIntLike(dev),
			"aws.neuron.device should be integer, got '%s'", dev)
		require.True(t, isIntLike(core),
			"aws.neuron.core should be integer, got '%s'", core)
	}
}

// TestMultiNeuronConsecutiveDeviceIndices validates indices are 0..N-1.
func TestMultiNeuronConsecutiveDeviceIndices(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0, "No results from %s node", multiDeviceInstanceType)

	devices := uniqueDatapointValuesList(multi, "aws.neuron.device")
	require.Equal(t, expectedMultiNeuronDeviceCount, len(devices), fmt.Sprintf("Expected %d Neuron device indices, got %d: %v",
		expectedMultiNeuronDeviceCount, len(devices), devices))
	for i := 0; i < expectedMultiNeuronDeviceCount; i++ {
		expected := strconv.Itoa(i)
		found := false
		for _, d := range devices {
			d := d
			if d == expected {
				found = true
				break
			}
		}
		require.True(t, found,
			"Expected Neuron device index %d, got indices: %v", i, devices)
	}
}

// TestMultiNeuronAllMetricsPerDevice validates that every core-level Neuron
// metric produces the expected per-device/per-core cardinality on the
// multi-device node.
func TestMultiNeuronAllMetricsPerDevice(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			multi := filterByHostType(results, multiDeviceInstanceType)
			require.True(t, len(multi) > 0,
				"No %s results from %s node", md.Name, multiDeviceInstanceType)

			// Some metrics have (device, core), others only device.
			pairs := uniqueDatapointPairs(multi, "aws.neuron.device", "aws.neuron.core")
			if len(pairs) > 0 {
				require.Equal(t, expectedMultiNeuronCoreCount, len(pairs), fmt.Sprintf("%s: expected %d (device, core) pairs, got %d",
					md.Name, expectedMultiNeuronCoreCount, len(pairs)))
			} else {
				devices := uniqueDatapointValuesList(multi, "aws.neuron.device")
				require.Equal(t, expectedMultiNeuronDeviceCount, len(devices), fmt.Sprintf("%s: expected %d devices, got %d: %v",
					md.Name, expectedMultiNeuronDeviceCount, len(devices), devices))
			}
		})
	}
}

// TestMultiNeuronHostType validates host.type label is present and matches.
func TestMultiNeuronHostType(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")

	multi := filterByHostType(results, multiDeviceInstanceType)
	require.True(t, len(multi) > 0,
		"No neuroncore_utilization_ratio results from %s", multiDeviceInstanceType)
	for _, r := range multi {
		r := r
		require.Equal(t, multiDeviceInstanceType, r.Labels.Resource["host.type"], "host.type on multi-device result")
	}
}

// ---------------------------------------------------------------------------
// Pod correlation tests
// ---------------------------------------------------------------------------

// TestMultiNeuronCorrelatedCoreCount validates that exactly 2 cores are
// correlated to the neuron-burn-multi pod (1 whole device × 2 cores/device).
// Uses a targeted PromQL query by pod name to avoid stale Zeus data.
func TestMultiNeuronCorrelatedCoreCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	escaped := escapePromQL(cfg.ClusterName)
	promql := fmt.Sprintf(
		`neuroncore_utilization_ratio{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"neuron-burn-multi.*"}`,
		escaped)
	burnResults, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying neuroncore_utilization_ratio for neuron-burn-multi")
	require.Equal(t, 2, len(burnResults), fmt.Sprintf("Expected 2 cores correlated to neuron-burn-multi (1 whole device × 2 cores), got %d",
		len(burnResults)))
}

// TestMultiNeuronBurnDevicePodLabels validates labels on the multi-device burn pod.
// The pod requests 1 whole device (aws.amazon.com/neuron: "1"), so it gets
// 2 cores all on the same device.
func TestMultiNeuronBurnDevicePodLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	escaped := escapePromQL(cfg.ClusterName)
	promql := fmt.Sprintf(
		`neuroncore_utilization_ratio{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"neuron-burn-multi.*"}`,
		escaped)
	burnResults, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying neuroncore_utilization_ratio for neuron-burn-multi")
	require.Equal(t, 2, len(burnResults), fmt.Sprintf("Expected 2 cores correlated to neuron-burn-multi, got %d", len(burnResults)))

	devices := make(map[string]struct{})
	for _, r := range burnResults {
		r := r
		devices[r.Labels.Datapoint["aws.neuron.device"]] = struct{}{}
		require.Equal(t, "neuron-burn", r.Labels.Resource["k8s.container.name"], "neuron-burn-multi container name")
		require.Equal(t, "neuron-burn-multi", r.Labels.Resource["k8s.workload.name"], "neuron-burn-multi workload name")
	}
	require.Equal(t, 1, len(devices), fmt.Sprintf("Expected neuron-burn-multi cores on 1 device, got %d", len(devices)))
}

// ---------------------------------------------------------------------------
// Anti-regression
// ---------------------------------------------------------------------------

// TestNeuronNoDuplicateSeries validates the dedup processor removes duplicates.
// Ports from monorepo's TestNeuronNoDuplicateSeries.
func TestNeuronNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	for _, md := range neuronCoreLevelMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)

			// Build a set of (labels) → count; any repeat is a duplicate.
			seen := make(map[string]int)
			for _, r := range results {
				r := r
				key := seriesKey(r)
				seen[key]++
			}
			for key, count := range seen {
				require.Equal(t, 1, count, "%s has duplicate series: %s (%d copies)", md.Name, key, count)
			}
		})
	}
}

// seriesKey constructs a stable string from all resource + datapoint labels.
func seriesKey(r otelmetrics.MetricResult) string {
	var b strings.Builder
	keys := make([]string, 0, len(r.Labels.Resource))
	for k := range r.Labels.Resource {
		keys = append(keys, k)
	}
	// deterministic order
	sort.Strings(keys)
	for _, k := range keys {
		k := k
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(r.Labels.Resource[k])
		b.WriteString(";")
	}
	b.WriteString("|DP|")
	dkeys := make([]string, 0, len(r.Labels.Datapoint))
	for k := range r.Labels.Datapoint {
		dkeys = append(dkeys, k)
	}
	sort.Strings(dkeys)
	for _, k := range dkeys {
		k := k
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(r.Labels.Datapoint[k])
		b.WriteString(";")
	}
	return b.String()
}
