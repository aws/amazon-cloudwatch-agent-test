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

// TestTargetAllocatorHealthyOnBundledInstall is a smoke test: with the chart's
// bundled ServiceMonitor/PodMonitor CRDs installed, the Target Allocator comes up
// and stays healthy (Available, Ready, not CrashLoopBackOff).
//
// It does NOT exercise the missing-CRD install-ordering window: the harness
// bundles the CRDs at install time, so they are present when the TA starts, and a
// rollout restart would not reopen that window (it would also zero the lifetime
// restart count). The missing-CRD tolerance itself (G2 resilience) is covered by
// the operator's Target Allocator unit tests; here we only assert the bundled
// install is healthy.
func TestTargetAllocatorHealthyOnBundledInstall(t *testing.T) {
	clientset := k8sClientset(t)

	dep := targetAllocatorDeployment(t, clientset)

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	require.Positive(t, desired, "Target Allocator has 0 desired replicas")
	require.Equal(t, desired, dep.Status.ReadyReplicas,
		"Target Allocator not fully ready: ready=%d desired=%d (a crashloop would look like this)",
		dep.Status.ReadyReplicas, desired)

	assert.True(t, deploymentAvailable(dep), "Target Allocator Deployment Available condition is not True")

	pods := targetAllocatorPods(t, clientset)
	require.NotEmpty(t, pods, "no Target Allocator pods found with the component label")
	for _, p := range pods {
		assert.Equalf(t, corev1.PodRunning, p.Status.Phase, "TA pod %s phase=%s", p.Name, p.Status.Phase)
		// Smoke signal: the TA is up and not crashlooping. (Lifetime restart count
		// is intentionally not asserted here: it is only a clean signal on a freshly
		// provisioned cluster, so the harness additionally logs it below.)
		for _, cs := range p.Status.ContainerStatuses {
			assert.Truef(t, cs.Ready, "TA pod %s container %s is not Ready", p.Name, cs.Name)
			assert.NotNilf(t, cs.State.Running, "TA pod %s container %s is not Running (state=%+v)", p.Name, cs.Name, cs.State)
			if cs.State.Waiting != nil {
				assert.NotEqualf(t, "CrashLoopBackOff", cs.State.Waiting.Reason,
					"TA pod %s container %s is in CrashLoopBackOff", p.Name, cs.Name)
			}
		}
	}

	// Informational: on a freshly provisioned cluster this should be 0.
	t.Logf("Target Allocator lifetime container restarts: %d (expected 0 on a fresh harness cluster)", totalRestarts(pods))
}

// TestTargetAllocatorDiscoversMonitors confirms that with the bundled CRDs
// present the TA actually discovers ServiceMonitor/PodMonitor targets, proving
// the CRD watch started the informers (rather than the TA merely surviving).
// Presence of the per-node workload metrics in CloudWatch is the end-to-end
// signal that discovery -> allocation -> scrape -> export all work.
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
