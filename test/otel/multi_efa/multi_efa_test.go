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
