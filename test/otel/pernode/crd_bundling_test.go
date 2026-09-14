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
// ServiceMonitor and PodMonitor CRDs are present AND owned by this chart's Helm
// release. Checking Helm ownership (app.kubernetes.io/managed-by=Helm) rather than
// mere presence means a rerun or a pre-existing prometheus-operator can't make the
// test pass when the chart bundled nothing. CRD absence is treated as a
// precondition failure (the harness installs onto a cluster without these CRDs).
func TestServiceMonitorPodMonitorCRDsBundled(t *testing.T) {
	dyn := dynamicClient(t)

	smPresent, smHelm, smNS := crdManagedByHelm(t, dyn, gvrServiceMonitor)
	pmPresent, pmHelm, pmNS := crdManagedByHelm(t, dyn, gvrPodMonitor)

	// Absence => the chart didn't bundle them: precondition for the ownership check.
	require.Truef(t, smPresent, "ServiceMonitor CRD absent — chart did not bundle it (zero-step install precondition)")
	require.Truef(t, pmPresent, "PodMonitor CRD absent — chart did not bundle it (zero-step install precondition)")

	// Presence alone is insufficient (a pre-existing prometheus-operator or a stale
	// prior install would also be present); require THIS Helm release to own them.
	assert.Truef(t, smHelm,
		"ServiceMonitor CRD is not managed by Helm (app.kubernetes.io/managed-by != Helm); not installed by this chart (release-namespace=%q)", smNS)
	assert.Truef(t, pmHelm,
		"PodMonitor CRD is not managed by Helm (app.kubernetes.io/managed-by != Helm); not installed by this chart (release-namespace=%q)", pmNS)
	assert.Equalf(t, agentNamespace, smNS, "ServiceMonitor CRD Helm release-namespace=%q, want %q (this release)", smNS, agentNamespace)
	assert.Equalf(t, agentNamespace, pmNS, "PodMonitor CRD Helm release-namespace=%q, want %q (this release)", pmNS, agentNamespace)
}
