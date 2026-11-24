//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Host attribute validation tests cross-check that resourcedetection attributes
// (host.id, host.name, cloud.*) are correct for the node each metric came
// from, not just present. They cover the DaemonSet metrics (local resourcedetection).
package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// perHostMetric runs check on every series of every host-enriched metric.
func perHostMetric(t *testing.T, check func(t *testing.T, metricName string, labels map[string]string)) {
	t.Helper()
	for _, metricName := range hostEnrichedMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				check(t, metricName, r.Labels.Resource)
			}
		})
	}
}

// TestHostTypeMatchesNodeGroup is skipped: GKE's gcp resourcedetection processor has no
// machine-type resource attribute (no equivalent of AKS's azure.vm.size), confirmed absent on a real
// run alongside cloud.region -- see kubeletstats_test.go's TestKubeletstatsHostDetection.
func TestHostTypeMatchesNodeGroup(t *testing.T) {
	t.Skip("GKE: no machine-type resource attribute exists to cross-check (no azure.vm.size equivalent)")
}

// TestHostNameMatchesNodeName — on GKE host.name is the node name.
func TestHostNameMatchesNodeName(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			matchCount := 0
			for _, r := range results {
				hostName := r.Labels.Resource["host.name"]
				nodeName := r.Labels.Resource["k8s.node.name"]
				if hostName == "" || nodeName == "" {
					continue
				}
				matchCount++
				require.Equal(t, nodeName, hostName,
					"%s host.name should match k8s.node.name", metricName)
			}
			require.True(t, matchCount > 0,
				"%s: no results had both host.name and k8s.node.name — enrichment may have failed entirely", metricName)
		})
	}
}

// TestUniqueHostIdPerNode — each distinct k8s.node.name must have a distinct host.id.
// The standard GKE cluster has one node, so at least one node is required, not two.
func TestUniqueHostIdPerNode(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_cpu_seconds_total")
	require.NoError(t, err, "querying node_cpu_seconds_total")
	require.NotEmpty(t, results, "node_cpu_seconds_total not available")

	nodeToHostID := make(map[string]string)
	hostIDToNode := make(map[string]string)
	for _, r := range results {
		node := r.Labels.Resource["k8s.node.name"]
		hostID := r.Labels.Resource["host.id"]
		if node == "" || hostID == "" {
			continue
		}
		if existing, seen := nodeToHostID[node]; seen {
			require.Equal(t, existing, hostID, fmt.Sprintf("node %s has inconsistent host.id", node))
			continue
		}
		if otherNode, taken := hostIDToNode[hostID]; taken {
			t.Fatalf("host.id %s is shared by nodes %s and %s", hostID, otherNode, node)
		}
		nodeToHostID[node] = hostID
		hostIDToNode[hostID] = node
	}
	require.True(t, len(nodeToHostID) >= 1, "Expected at least 1 node with host.id, got %d", len(nodeToHostID))
}

// TestCloudRegionMatchesConfig is skipped: a real run (45377121910, 2026-11-24) confirmed cloud.region
// is absent from GKE resource attributes entirely, not just unequal to the node's region label -- see
// kubeletstats_test.go's TestKubeletstatsCloudDetection and ksm_test.go's requireGKECloudAttributes.
func TestCloudRegionMatchesConfig(t *testing.T) {
	t.Skip("GKE: cloud.region is confirmed absent from resource attributes on a real run, nothing to compare")
}

// TestCloudAccountMatchesConfig — cloud.account.id is the GCP project ID and must be the one
// inside cloud.resource_id (//container.googleapis.com/projects/<project>/...).
func TestCloudAccountMatchesConfig(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		acct, id := l["cloud.account.id"], l["cloud.resource_id"]
		if acct == "" {
			return
		}
		require.True(t, strings.Contains(id, "/projects/"+acct+"/"),
			"%s cloud.account.id=%s is not the project of cloud.resource_id=%s", metricName, acct, id)
	})
}

// TestCloudAvailabilityZonePresent — unlike AKS (non-zonal cluster), the standard GKE cluster is zonal
// and the chart's gcp resourcedetection processor does set cloud.availability_zone (confirmed on a real
// run and already asserted node-by-node in ksm_test.go's requireGKECloudAttributes).
func TestCloudAvailabilityZonePresent(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		zone, ok := l["cloud.availability_zone"]
		require.True(t, ok, "%s missing @resource.cloud.availability_zone", metricName)
		require.True(t, zone != "", "%s has empty @resource.cloud.availability_zone", metricName)
	})
}

// TestCloudProviderIsAWS (GCP: provider must be "gcp").
func TestCloudProviderIsAWS(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		require.Equal(t, "gcp", l["cloud.provider"], "%s cloud.provider", metricName)
	})
}

// TestCloudPlatformIsEKS (GCP: platform must be gcp_kubernetes_engine).
func TestCloudPlatformIsEKS(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		p := l["cloud.platform"]
		require.True(t, p == "gcp_kubernetes_engine", "%s cloud.platform=%q, want gcp_kubernetes_engine", metricName, p)
	})
}

// TestCloudResourceIdIsEKSArn (GCP: cloud.resource_id is the GKE cluster's full resource name).
func TestCloudResourceIdIsEKSArn(t *testing.T) {
	prefix := "//container.googleapis.com/projects/"
	suffix := "/clusters/" + cfg.ClusterName
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		id := l["cloud.resource_id"]
		require.True(t, id != "", "%s missing cloud.resource_id", metricName)
		require.True(t, strings.HasPrefix(id, prefix), "%s cloud.resource_id=%s", metricName, id)
		require.True(t, strings.HasSuffix(id, suffix), "%s cloud.resource_id=%s should end with %s", metricName, id, suffix)
	})
}

// ===========================================================================
// K8s Ground Truth Validation Tests
// ===========================================================================

// ---------------------------------------------------------------------------
// TestPodScheduledOnCorrectNode — verify the pod is actually scheduled on
// the node reported in the metric.
// ---------------------------------------------------------------------------

func TestPodScheduledOnCorrectNode(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range podScopedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				nodeName := r.Labels.Resource["k8s.node.name"]
				if podName == "" || nodeName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				if pod.Spec.NodeName != nodeName {
					t.Errorf("%s pod %s: metric says node=%s, K8s API says node=%s",
						metricName, podName, nodeName, pod.Spec.NodeName)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestHostIdMatchesProviderID — on GKE host.id is the VM's system UUID, which the node reports in
// status.nodeInfo.systemUUID (spec.providerID is a GCE instance path, not the ID).
// ---------------------------------------------------------------------------

func TestHostIdMatchesProviderID(t *testing.T) {
	gt := getGroundTruth(t)
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		nodeName, hostID := l["k8s.node.name"], l["host.id"]
		if nodeName == "" || hostID == "" {
			return
		}
		node, found := gt.nodes[nodeName]
		if !found {
			return
		}
		require.True(t, strings.EqualFold(node.Status.NodeInfo.SystemUUID, hostID),
			"%s node %s: metric host.id=%s, node systemUUID=%s", metricName, nodeName, hostID, node.Status.NodeInfo.SystemUUID)
	})
}

// ---------------------------------------------------------------------------
// TestNodeUidMatchesKubernetesAPI — verify k8s.node.uid matches the K8s API.
// ---------------------------------------------------------------------------

func TestNodeUidMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				nodeName := r.Labels.Resource["k8s.node.name"]
				nodeUID := r.Labels.Resource["k8s.node.uid"]
				if nodeName == "" || nodeUID == "" {
					continue
				}
				node, found := gt.nodes[nodeName]
				if !found {
					continue
				}
				if string(node.UID) != nodeUID {
					t.Errorf("%s node %s: metric k8s.node.uid=%s, K8s API uid=%s",
						metricName, nodeName, nodeUID, string(node.UID))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestContainerExistsInPodSpec — verify the container actually exists in the
// pod spec.
// ---------------------------------------------------------------------------

func TestContainerExistsInPodSpec(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range podScopedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				containerName := r.Labels.Resource["k8s.container.name"]
				if podName == "" || containerName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				exists := false
				for _, c := range pod.Spec.Containers {
					if c.Name == containerName {
						exists = true
						break
					}
				}
				if !exists {
					for _, c := range pod.Spec.InitContainers {
						if c.Name == containerName {
							exists = true
							break
						}
					}
				}
				if !exists {
					t.Errorf("%s pod %s: container %q not found in pod spec",
						metricName, podName, containerName)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPodNamespaceMatchesKubernetesAPI — verify the namespace matches the
// K8s API.
// ---------------------------------------------------------------------------

func TestPodNamespaceMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				ns := r.Labels.Resource["k8s.namespace.name"]
				if podName == "" || ns == "" {
					continue
				}
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				if pod.Namespace != ns {
					t.Errorf("%s pod %s: metric namespace=%s, K8s API namespace=%s",
						metricName, podName, ns, pod.Namespace)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNodeLabelsMatchKubernetesAPI — verify k8s.node.label.* resource
// attributes match the actual node labels from the K8s API.
// ---------------------------------------------------------------------------

func TestNodeLabelsMatchKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	const labelPrefix = "k8s.node.label."
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				nodeName := r.Labels.Resource["k8s.node.name"]
				if nodeName == "" {
					continue
				}
				node, found := gt.nodes[nodeName]
				if !found {
					continue
				}
				for attrKey, attrVal := range r.Labels.Resource {
					if !strings.HasPrefix(attrKey, labelPrefix) {
						continue
					}
					labelKey := strings.TrimPrefix(attrKey, labelPrefix)
					k8sVal, exists := node.Labels[labelKey]
					if !exists {
						t.Errorf("%s node %s: metric has label %s=%s but K8s node has no label %s",
							metricName, nodeName, attrKey, attrVal, labelKey)
						continue
					}
					if k8sVal != attrVal {
						t.Errorf("%s node %s: label %s metric=%s, K8s API=%s",
							metricName, nodeName, labelKey, attrVal, k8sVal)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDeploymentOwnerMatchesKubernetesAPI — verify the pod's owner chain
// includes a ReplicaSet named <deployment>-<hash>.
// ---------------------------------------------------------------------------

func TestDeploymentOwnerMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				deployName := r.Labels.Resource["k8s.deployment.name"]
				if podName == "" || deployName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				if len(pod.OwnerReferences) == 0 {
					continue
				}
				var rsName string
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "ReplicaSet" {
						rsName = ref.Name
						break
					}
				}
				if rsName == "" {
					continue
				}
				expectedPrefix := deployName + "-"
				if !strings.HasPrefix(rsName, expectedPrefix) {
					t.Errorf("%s pod %s: deployment=%s but ReplicaSet owner=%s (expected prefix %s)",
						metricName, podName, deployName, rsName, expectedPrefix)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestStatefulSetOwnerMatchesKubernetesAPI — verify the pod's ownerReferences
// include a StatefulSet with the expected name.
// ---------------------------------------------------------------------------

func TestStatefulSetOwnerMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				stsName := r.Labels.Resource["k8s.statefulset.name"]
				if podName == "" || stsName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				var ownerSts string
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "StatefulSet" {
						ownerSts = ref.Name
						break
					}
				}
				require.True(t, ownerSts != "",
					"%s pod %s: has k8s.statefulset.name=%s but no StatefulSet ownerReference",
					metricName, podName, stsName)
				require.Equal(t, stsName, ownerSts,
					"%s pod %s: k8s.statefulset.name=%s but ownerReference=%s",
					metricName, podName, stsName, ownerSts)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDaemonSetOwnerMatchesKubernetesAPI — verify the pod's ownerReferences
// include a DaemonSet with the expected name.
// ---------------------------------------------------------------------------

func TestDaemonSetOwnerMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				dsName := r.Labels.Resource["k8s.daemonset.name"]
				if podName == "" || dsName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				var ownerDS string
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "DaemonSet" {
						ownerDS = ref.Name
						break
					}
				}
				require.True(t, ownerDS != "",
					"%s pod %s: has k8s.daemonset.name=%s but no DaemonSet ownerReference",
					metricName, podName, dsName)
				require.Equal(t, dsName, ownerDS,
					"%s pod %s: k8s.daemonset.name=%s but ownerReference=%s",
					metricName, podName, dsName, ownerDS)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestJobOwnerMatchesKubernetesAPI — verify the pod's ownerReferences
// include a Job with the expected name.
// ---------------------------------------------------------------------------

func TestJobOwnerMatchesKubernetesAPI(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range daemonsetMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				podName := r.Labels.Resource["k8s.pod.name"]
				jobName := r.Labels.Resource["k8s.job.name"]
				if podName == "" || jobName == "" {
					continue
				}
				ns := r.Labels.Resource["k8s.namespace.name"]
				pod, found := gt.lookupPod(podName, ns)
				if !found {
					continue
				}
				var ownerJob string
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "Job" {
						ownerJob = ref.Name
						break
					}
				}
				require.True(t, ownerJob != "",
					"%s pod %s: has k8s.job.name=%s but no Job ownerReference",
					metricName, podName, jobName)
				require.Equal(t, jobName, ownerJob,
					"%s pod %s: k8s.job.name=%s but ownerReference=%s",
					metricName, podName, jobName, ownerJob)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAllNodeGroupsPresent — the default node pool must have at least one node reporting metrics.
// GKE names its default pool's nodes "gke-<cluster-name>-default-pool-<hash>-<id>" (see
// kubeletstats_test.go's TestKubeletstatsNodeGroupCoverage); there's no node-pool label equivalent to
// AKS's kubernetes.azure.com/agentpool to filter the query on, so this matches on node name instead.
// ---------------------------------------------------------------------------

func TestAllNodeGroupsPresent(t *testing.T) {
	results, err := queryCache.Get(context.Background(), "node_load1")
	require.NoError(t, err, "querying node_load1")
	require.NotEmpty(t, results, "node_load1 not available")

	found := 0
	for _, r := range results {
		if strings.Contains(r.Labels.Resource["k8s.node.name"], "default-pool") {
			found++
		}
	}
	require.True(t, found > 0, "no metrics from the default-pool nodes — it may have been deleted")
}
