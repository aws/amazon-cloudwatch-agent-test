//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceMonitorPodMonitorCRDsBundled verifies G1 (zero-step install): after
// installing the chart with otelContainerInsights enabled, the community
// ServiceMonitor and PodMonitor CRDs are present and established (served by the
// API server) with no manual prerequisite. The otel-pernode harness installs the
// chart onto a cluster that does NOT pre-install these CRDs, so their presence
// here is attributable to the chart's bundling.
func TestServiceMonitorPodMonitorCRDsBundled(t *testing.T) {
	clientset := k8sClientset(t)

	smServed := crdServed(t, clientset, gvrServiceMonitor.GroupVersion().String(), gvrServiceMonitor.Resource)
	pmServed := crdServed(t, clientset, gvrPodMonitor.GroupVersion().String(), gvrPodMonitor.Resource)

	assert.True(t, smServed, "ServiceMonitor CRD (%s/%s) is not established; chart did not bundle it",
		gvrServiceMonitor.GroupVersion().String(), gvrServiceMonitor.Resource)
	assert.True(t, pmServed, "PodMonitor CRD (%s/%s) is not established; chart did not bundle it",
		gvrPodMonitor.GroupVersion().String(), gvrPodMonitor.Resource)

	require.True(t, smServed && pmServed,
		"zero-step CRD bundling failed: ServiceMonitor served=%v, PodMonitor served=%v", smServed, pmServed)
}
