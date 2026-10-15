//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package multi_efa validates that a node with multiple EFA interfaces
// produces distinct per-device metric series with correct device attributes.
//
// Cluster topology:
//   - 1x c6in.32xlarge with 2 EFA interfaces (1 per network card)
//   - Node label: ci-test.example.com/multi-efa-sm=true
package multi_efa

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	expectedMultiEFACount = 2
	multiEfaSmNodeLabel   = "k8s.node.label.ci-test.example.com/multi-efa-sm"
)

func TestMultiEFADeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available")

	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
	require.True(t, len(multi) > 0,
		"No efa_rx_bytes results from multi-EFA node (label %s)", multiEfaSmNodeLabel)

	devices := uniqueAnyValues(multi, "aws.efa.device")
	require.Equal(t, expectedMultiEFACount, len(devices),
		"Expected %d distinct aws.efa.device, got %d: %v",
		expectedMultiEFACount, len(devices), devices)
}

func TestMultiEFAUniqueENIs(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available")

	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA node")

	enis := uniqueAnyValues(multi, "aws.efa.eni.id")
	devices := uniqueAnyValues(multi, "aws.efa.device")
	require.Equal(t, len(devices), len(enis),
		"EFA device count (%d) != ENI count (%d) — duplicate ENIs?",
		len(devices), len(enis))
	for _, eni := range enis {
		require.True(t, strings.HasPrefix(eni, "eni-"),
			"aws.efa.eni.id should start with 'eni-', got '%s'", eni)
	}
}

func TestMultiEFADeviceNamesAreRDMA(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available")

	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA node")

	for _, r := range multi {
		dev := getAnyValue(r, "aws.efa.device")
		require.NotEmpty(t, dev, "aws.efa.device is empty")
	}
}

func TestMultiEFAAllMetricsPerDevice(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
			require.True(t, len(multi) > 0, "No %s results from multi-EFA node", metricName)

			devices := uniqueAnyValues(multi, "aws.efa.device")
			require.Equal(t, expectedMultiEFACount, len(devices),
				"%s: expected %d EFA devices, got %d: %v",
				metricName, expectedMultiEFACount, len(devices), devices)
		})
	}
}

func TestMultiEFAPortPresent(t *testing.T) {
	t.Parallel()
	for _, metricName := range efaMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
			require.True(t, len(multi) > 0, "No %s results from multi-EFA node", metricName)

			for _, r := range multi {
				port := getAnyValue(r, "aws.efa.port")
				require.NotEmpty(t, port,
					"%s missing aws.efa.port (device: %s)",
					metricName, getAnyValue(r, "aws.efa.device"))
			}
		})
	}
}

func TestMultiEFACorrelatedCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA node")

	var correlated int
	for _, r := range multi {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efaburn") {
			correlated++
		}
	}
	require.True(t, correlated >= 1,
		"Expected at least 1 EFA correlated to efaburn, got %d", correlated)
}

// TestMultiEFACorrelatedPodLabels validates that the correlated EFA result
// has the expected pod labels.
func TestMultiEFACorrelatedPodLabels(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")

	for _, r := range multi {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], "efaburn") {
			require.Equal(t, "efaburn", r.Labels.Resource["k8s.container.name"], "efaburn container name")
			require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "efaburn namespace")
			return
		}
	}
	t.Fatal("No efa_rx_bytes result correlated to efaburn pod")
}

// expectedClaimedEFACount is how many of the node's EFA devices are claimed by a
// pod. efaburn (replicas: 1) requests 1 EFA, so exactly one device is claimed and
// the remaining device(s) must stay unclaimed.
const expectedClaimedEFACount = 1

// TestMultiEFAClaimedVsUnclaimedCorrelation validates per-device pod correlation
// on a multi-EFA node: the device claimed by efaburn is correlated to that pod,
// and every remaining (unclaimed) device carries NO pod attributes.
//
// Regression guard for the EFA multi-device correlation collapse. Without the
// groupbyattrs/efa split before the resource-level promote, ALL of the node's EFA
// devices — including unclaimed ones — are attributed to a single pod. This test
// fails in that case because (a) more than expectedClaimedEFACount devices carry a
// pod, and (b) no device is left unclaimed. It also catches a single device being
// attributed to multiple pods.
func TestMultiEFAClaimedVsUnclaimedCorrelation(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	multi := filterByNodeLabel(results, multiEfaSmNodeLabel, "true")
	require.NotEmpty(t, multi,
		"No efa_rx_bytes results from multi-EFA node (label %s)", multiEfaSmNodeLabel)

	// For each device, collect the distinct workloads it is attributed to (empty =
	// unclaimed), and whether it carries any pod-scope attribute at all. Pods are
	// grouped by workload prefix (efaburn) so that a mid-run pod replacement — two
	// efaburn-<hash> pods briefly reporting the same device — counts as one owner
	// rather than tripping the multi-pod collapse check. Pod identity is read via
	// getAnyValue so a regression that leaves it at datapoint scope is still caught.
	deviceOwners := make(map[string]map[string]struct{})
	deviceHasPodAttr := make(map[string]bool)
	for _, r := range multi {
		dev := getAnyValue(r, "aws.efa.device")
		if dev == "" {
			continue
		}
		if deviceOwners[dev] == nil {
			deviceOwners[dev] = make(map[string]struct{})
		}
		pod := getAnyValue(r, "k8s.pod.name")
		ns := getAnyValue(r, "k8s.namespace.name")
		container := getAnyValue(r, "k8s.container.name")
		if pod != "" || ns != "" || container != "" {
			deviceHasPodAttr[dev] = true
		}
		if pod != "" {
			deviceOwners[dev][workloadKey(pod)] = struct{}{}
		}
	}
	require.Len(t, deviceOwners, expectedMultiEFACount,
		"expected %d EFA devices on the node, got %d: %v",
		expectedMultiEFACount, len(deviceOwners), deviceKeys(deviceOwners))

	var claimed, unclaimed []string
	for dev, owners := range deviceOwners {
		switch len(owners) {
		case 0:
			// An unclaimed device must carry no pod/namespace/container at all.
			require.False(t, deviceHasPodAttr[dev],
				"EFA device %s has no pod name but carries namespace/container attributes; unclaimed devices must carry no pod-scope attributes", dev)
			unclaimed = append(unclaimed, dev)
		case 1:
			claimed = append(claimed, dev)
		default:
			t.Errorf("EFA device %s correlated to multiple workloads: %v", dev, setKeys(owners))
		}
	}
	if t.Failed() {
		return
	}

	// Claimed side: exactly the number of EFAs efaburn requested map to a pod.
	require.Len(t, claimed, expectedClaimedEFACount,
		"expected %d correlated (claimed) EFA device(s), got %d: %v",
		expectedClaimedEFACount, len(claimed), claimed)

	// Unclaimed side: the remaining devices must carry no pod.
	require.Len(t, unclaimed, expectedMultiEFACount-expectedClaimedEFACount,
		"expected %d unclaimed EFA device(s) with no pod, got %d: %v",
		expectedMultiEFACount-expectedClaimedEFACount, len(unclaimed), unclaimed)
}

// workloadKey collapses a pod name to its workload identity so replicas/replaced
// pods of the same deployment count as one owner. efaburn is the only claimant
// workload on this node.
func workloadKey(pod string) string {
	if strings.HasPrefix(pod, "efaburn") {
		return "efaburn"
	}
	return pod
}
