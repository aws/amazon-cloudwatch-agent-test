//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package multi_efa_dra validates per-device EFA metric correlation on the
// Dynamic Resource Allocation (DRA) path — the same behavior the multi_efa
// package validates for the EFA device-plugin path, but with EFA devices
// allocated via DRA (the dranet driver, driver name "dra.net") through a
// ResourceClaimTemplate instead of the vpc.amazonaws.com/efa device-plugin
// resource.
//
// This exercises the awsdevicepodcorrelation processor's DRA path end to end:
// the processor watches Pods/ResourceClaims/ResourceSlices via the K8s API and
// bridges the DRA device identity (a PCI name) to the EFA metric label
// (rdmapXsY) via the ResourceSlice attribute dra.net/rdmaDevice. The emitted
// metrics are identical to the device-plugin path, so the assertions mirror the
// multi_efa package.
//
// Cluster topology:
//   - 1x c6in.32xlarge with 2 EFA interfaces (1 per network card)
//   - EFA exposed via DRA (dranet), not the EFA device plugin
//   - Node label: ci-test.example.com/multi-efa-dra=true
package multi_efa_dra

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	expectedMultiEFACount = 2
	multiEfaDraNodeLabel  = "k8s.node.label.ci-test.example.com/multi-efa-dra"
)

func TestMultiEFADeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	require.NotEmpty(t, results, "efa_rx_bytes not available")

	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
	require.True(t, len(multi) > 0,
		"No efa_rx_bytes results from multi-EFA DRA node (label %s)", multiEfaDraNodeLabel)

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

	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA DRA node")

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

	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA DRA node")

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
			multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
			require.True(t, len(multi) > 0, "No %s results from multi-EFA DRA node", metricName)

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
			multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
			require.True(t, len(multi) > 0, "No %s results from multi-EFA DRA node", metricName)

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
	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
	require.True(t, len(multi) > 0, "No results from multi-EFA DRA node")

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
	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")

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
// pod. efaburn (replicas: 1) requests 1 EFA via a ResourceClaimTemplate, so
// exactly one device is claimed and the remaining device(s) must stay unclaimed.
const expectedClaimedEFACount = 1

// TestMultiEFAClaimedVsUnclaimedCorrelation validates per-device pod correlation
// on a multi-EFA node whose devices are allocated via DRA: the device claimed by
// efaburn is correlated to that pod, and every remaining (unclaimed) device
// carries NO pod attributes.
//
// This is the DRA-path counterpart of the device-plugin regression guard. It
// exercises the processor's DRA correlation (ResourceClaim/ResourceSlice keying
// via dra.net/rdmaDevice) together with the groupbyattrs/efa split before the
// resource-level promote. Without correct per-device correlation, ALL of the
// node's EFA devices — including unclaimed ones — collapse onto a single pod;
// this test fails in that case because (a) more than expectedClaimedEFACount
// devices carry a pod, and (b) no device is left unclaimed. It also catches a
// single device attributed to multiple pods.
func TestMultiEFAClaimedVsUnclaimedCorrelation(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "efa_rx_bytes")
	require.NoError(t, err, "querying efa_rx_bytes")
	multi := filterByNodeLabel(results, multiEfaDraNodeLabel, "true")
	require.NotEmpty(t, multi,
		"No efa_rx_bytes results from multi-EFA DRA node (label %s)", multiEfaDraNodeLabel)

	// For each device, collect the distinct pods it is attributed to (empty = unclaimed).
	devicePods := make(map[string]map[string]struct{})
	for _, r := range multi {
		dev := getAnyValue(r, "aws.efa.device")
		if dev == "" {
			continue
		}
		if devicePods[dev] == nil {
			devicePods[dev] = make(map[string]struct{})
		}
		if pod := r.Labels.Resource["k8s.pod.name"]; pod != "" {
			devicePods[dev][pod] = struct{}{}
		}
	}
	require.Len(t, devicePods, expectedMultiEFACount,
		"expected %d EFA devices on the node, got %d: %v",
		expectedMultiEFACount, len(devicePods), deviceKeys(devicePods))

	var claimed, unclaimed []string
	for dev, pods := range devicePods {
		switch len(pods) {
		case 0:
			unclaimed = append(unclaimed, dev)
		case 1:
			claimed = append(claimed, dev)
		default:
			// A single device correlated to multiple pods is itself a collapse symptom.
			t.Errorf("EFA device %s correlated to multiple pods %v", dev, setKeys(pods))
		}
	}

	// Claimed side: exactly the number of EFAs efaburn requested map to a pod.
	require.Len(t, claimed, expectedClaimedEFACount,
		"expected %d correlated (claimed) EFA device(s), got %d: %v "+
			"(collapse over-correlates unclaimed devices onto a pod)",
		expectedClaimedEFACount, len(claimed), claimed)

	// Unclaimed side: the remaining devices must carry no pod — this is the
	// coverage that distinguishes correct correlation from the collapse.
	require.Len(t, unclaimed, expectedMultiEFACount-expectedClaimedEFACount,
		"expected %d unclaimed EFA device(s) with no pod, got %d: %v",
		expectedMultiEFACount-expectedClaimedEFACount, len(unclaimed), unclaimed)
}
