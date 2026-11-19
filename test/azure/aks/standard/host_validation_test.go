//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Host attribute validation tests cross-check that resourcedetection attributes
// (host.id, host.name, azure.vm.size, cloud.*) are correct for the node each metric came
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

// TestHostTypeMatchesNodeGroup — on AKS the VM size (azure.vm.size) must equal the node's
// node.kubernetes.io/instance-type label from the Kubernetes API.
func TestHostTypeMatchesNodeGroup(t *testing.T) {
	gt := getGroundTruth(t)
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		size, node := l["azure.vm.size"], l["k8s.node.name"]
		if size == "" || node == "" {
			return
		}
		n, found := gt.nodes[node]
		if !found {
			return
		}
		require.Equal(t, n.Labels["node.kubernetes.io/instance-type"], size,
			"%s azure.vm.size on node %s", metricName, node)
	})
}

// TestHostNameMatchesNodeName — on AKS host.name is the node name.
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
// The standard AKS cluster has one node, so at least one node is required, not two.
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

// TestCloudRegionMatchesConfig — cloud.region must equal the node's topology.kubernetes.io/region
// label (the Azure region; cfg.Region is the AWS region the metrics are stored in).
func TestCloudRegionMatchesConfig(t *testing.T) {
	gt := getGroundTruth(t)
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		region, node := l["cloud.region"], l["k8s.node.name"]
		if region == "" || node == "" {
			return
		}
		n, found := gt.nodes[node]
		if !found || n.Labels["topology.kubernetes.io/region"] == "" {
			return
		}
		require.Equal(t, n.Labels["topology.kubernetes.io/region"], region, "%s cloud.region", metricName)
	})
}

// TestCloudAccountMatchesConfig — cloud.account.id is the Azure subscription ID and must be the one
// inside cloud.resource_id.
func TestCloudAccountMatchesConfig(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		acct, id := l["cloud.account.id"], l["cloud.resource_id"]
		if acct == "" {
			return
		}
		require.True(t, strings.HasPrefix(id, "/subscriptions/"+acct+"/"),
			"%s cloud.account.id=%s is not the subscription of cloud.resource_id=%s", metricName, acct, id)
	})
}

// TestCloudAvailabilityZonePresent is skipped: the AKS agent does not emit cloud.availability_zone
// (AKS nodes in this cluster are not zonal).
func TestCloudAvailabilityZonePresent(t *testing.T) {
	t.Skip("AKS: cloud.availability_zone is not emitted on this non-zonal cluster")
}

// TestCloudProviderIsAWS (Azure: provider must be "azure").
func TestCloudProviderIsAWS(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		require.Equal(t, "azure", l["cloud.provider"], "%s cloud.provider", metricName)
	})
}

// TestCloudPlatformIsEKS (Azure: platform must be azure.aks).
func TestCloudPlatformIsEKS(t *testing.T) {
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		p := l["cloud.platform"]
		require.True(t, p == "azure.aks" || p == "azure_aks", "%s cloud.platform=%q, want azure.aks", metricName, p)
	})
}

// TestCloudResourceIdIsEKSArn (Azure: cloud.resource_id is the AKS managed cluster resource ID).
func TestCloudResourceIdIsEKSArn(t *testing.T) {
	suffix := "/managedClusters/" + cfg.ClusterName
	perHostMetric(t, func(t *testing.T, metricName string, l map[string]string) {
		id := l["cloud.resource_id"]
		require.True(t, id != "", "%s missing cloud.resource_id", metricName)
		require.True(t, strings.HasPrefix(id, "/subscriptions/"), "%s cloud.resource_id=%s", metricName, id)
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
// TestHostIdMatchesProviderID — on AKS host.id is the VM's system UUID, which the node reports in
// status.nodeInfo.systemUUID (spec.providerID is an Azure VMSS path, not the ID).
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
// ---------------------------------------------------------------------------

func TestAllNodeGroupsPresent(t *testing.T) {
	results, err := queryCache.GetWithFilter(context.Background(), "node_load1", map[string]string{
		"@resource.k8s.node.label.kubernetes.azure.com/agentpool": "default",
	})
	require.NoError(t, err, "querying node_load1 on the default pool")
	require.True(t, len(results) > 0, "no metrics from the default node pool — it may have been deleted")
}
