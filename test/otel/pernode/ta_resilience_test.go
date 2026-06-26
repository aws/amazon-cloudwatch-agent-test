//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTargetAllocatorHealthy verifies G2 (resilience to CRD install ordering):
// the Target Allocator becomes and stays healthy regardless of whether the
// ServiceMonitor/PodMonitor CRDs existed when it started. The harness installs
// the chart (which starts the TA) alongside CRD bundling, so the CRDs may become
// established after the TA process begins; the TA must NOT crashloop or fail
// readiness in that window.
//
// We assert the steady state: the TA Deployment is Available and its containers
// have not restarted (a crashloop on a missing CRD would show up as restarts).
func TestTargetAllocatorHealthy(t *testing.T) {
	clientset := k8sClientset(t)

	dep := targetAllocatorDeployment(t, clientset)

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	require.Positive(t, desired, "Target Allocator has 0 desired replicas")
	require.Equal(t, desired, dep.Status.ReadyReplicas,
		"Target Allocator not fully ready: ready=%d desired=%d (a crashloop on a missing CRD looks like this)",
		dep.Status.ReadyReplicas, desired)

	assert.True(t, deploymentAvailable(dep), "Target Allocator Deployment Available condition is not True")

	pods := targetAllocatorPods(t, clientset)
	require.NotEmpty(t, pods, "no Target Allocator pods found with the component label")
	for _, p := range pods {
		assert.Equalf(t, corev1.PodRunning, p.Status.Phase, "TA pod %s phase=%s", p.Name, p.Status.Phase)
	}

	restarts := totalRestarts(pods)
	assert.Zerof(t, restarts, "Target Allocator restarted %d time(s); expected 0 -- it should tolerate the "+
		"CRDs being absent at startup and pick them up without restarting", restarts)
}

// TestTargetAllocatorDiscoversMonitors confirms that once the CRDs are present
// the TA actually discovers ServiceMonitor/PodMonitor targets, proving the CRD
// watch started the informers (rather than the TA merely surviving). Presence of
// the per-node workload metrics in CloudWatch is the end-to-end signal that
// discovery -> allocation -> scrape -> export all work after the CRDs appeared.
func TestTargetAllocatorDiscoversMonitors(t *testing.T) {
	var found bool
	for _, m := range perNodeMetrics {
		if r := queryWorkloadMetric(t, m); len(r) > 0 {
			found = true
			break
		}
	}
	require.True(t, found,
		"no ServiceMonitor/PodMonitor-scraped metrics reached CloudWatch; the TA did not discover the monitors "+
			"after the CRDs became available")
}

func deploymentAvailable(dep *appsv1.Deployment) bool {
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
