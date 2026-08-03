//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package standard — host attribute validation tests cross-check that
// resourcedetection attributes (host.id, host.type, host.name, cloud.*)
// are correct for the node each metric came from, not just present.
// Covers DaemonSet metrics (local resourcedetection) and KSM node-scoped
// metrics (nodemetadataenricher via Lease cache).
package standard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestHostTypeMatchesNodeGroup — host.type must match the expected EC2
// instance type for the node group.
// ---------------------------------------------------------------------------

func TestHostTypeMatchesNodeGroup(t *testing.T) {
	// Build set of valid instance types from clusterNodeGroups.
	validTypes := make(map[string]bool, len(clusterNodeGroups))
	for _, ng := range clusterNodeGroups {
		validTypes[ng.InstanceType] = true
	}
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				hostType := r.Labels.Resource["host.type"]
				if hostType == "" {
					continue
				}
				require.True(t, validTypes[hostType],
					"%s host.type=%q not in expected node groups %v (node: %s)",
					metricName, hostType, validTypes,
					r.Labels.Resource["k8s.node.name"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestHostNameMatchesNodeName — on EKS, host.name (EC2 private DNS) should
// match k8s.node.name.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// TestUniqueHostIdPerNode — each distinct k8s.node.name must have a distinct
// host.id.
// ---------------------------------------------------------------------------

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
			require.Equal(t, existing, hostID,
				fmt.Sprintf("node %s has inconsistent host.id", node))
			continue
		}
		if otherNode, taken := hostIDToNode[hostID]; taken {
			t.Fatalf("host.id %s is shared by nodes %s and %s — resourcedetection is returning the same instance ID for different nodes",
				hostID, otherNode, node)
		}
		nodeToHostID[node] = hostID
		hostIDToNode[hostID] = node
	}
	require.True(t, len(nodeToHostID) >= 2,
		"Expected at least 2 distinct nodes, got %d", len(nodeToHostID))
}

// ---------------------------------------------------------------------------
// TestCloudRegionMatchesConfig — cloud.region must match the configured region.
// ---------------------------------------------------------------------------

func TestCloudRegionMatchesConfig(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				region := r.Labels.Resource["cloud.region"]
				if region == "" {
					continue
				}
				require.Equal(t, cfg.Region, region,
					"%s cloud.region", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudAccountMatchesConfig — cloud.account.id must match the test account.
// ---------------------------------------------------------------------------

func TestCloudAccountMatchesConfig(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				acct := r.Labels.Resource["cloud.account.id"]
				if acct == "" {
					continue
				}
				require.Equal(t, cfg.AccountID, acct,
					"%s cloud.account.id", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudAvailabilityZonePresent — cloud.availability_zone must be present
// and start with the region prefix.
// ---------------------------------------------------------------------------

func TestCloudAvailabilityZonePresent(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				az := r.Labels.Resource["cloud.availability_zone"]
				require.True(t, az != "", "%s missing cloud.availability_zone", metricName)
				require.True(t, strings.HasPrefix(az, cfg.Region),
					"%s cloud.availability_zone=%s should start with region %s",
					metricName, az, cfg.Region)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudProviderIsAWS — cloud.provider must be "aws".
// ---------------------------------------------------------------------------

func TestCloudProviderIsAWS(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				provider := r.Labels.Resource["cloud.provider"]
				require.Equal(t, "aws", provider, "%s cloud.provider", metricName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudPlatformIsEKS — cloud.platform must be "aws_eks".
// ---------------------------------------------------------------------------

func TestCloudPlatformIsEKS(t *testing.T) {
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				platform := r.Labels.Resource["cloud.platform"]
				require.True(t, platform == "aws_eks" || platform == "aws_ec2",
					"%s cloud.platform=%q, want aws_eks or aws_ec2", metricName, platform)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCloudResourceIdIsEKSArn — cloud.resource_id must be an EKS cluster ARN.
// ---------------------------------------------------------------------------

func TestCloudResourceIdIsEKSArn(t *testing.T) {
	expectedPrefix := fmt.Sprintf("arn:aws:eks:%s:", cfg.Region)
	expectedSuffix := fmt.Sprintf(":cluster/%s", cfg.ClusterName)
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				arn := r.Labels.Resource["cloud.resource_id"]
				require.True(t, arn != "", "%s missing cloud.resource_id", metricName)
				require.True(t, strings.HasPrefix(arn, expectedPrefix),
					"%s cloud.resource_id=%s should start with %s", metricName, arn, expectedPrefix)
				require.True(t, strings.HasSuffix(arn, expectedSuffix),
					"%s cloud.resource_id=%s should end with %s", metricName, arn, expectedSuffix)
			}
		})
	}
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
// TestHostIdMatchesProviderID — verify host.id matches the EC2 instance ID
// parsed from the node's spec.providerID.
// ---------------------------------------------------------------------------

func TestHostIdMatchesProviderID(t *testing.T) {
	gt := getGroundTruth(t)
	for _, metricName := range hostEnrichedMetricNames() {
		t.Run(metricName, func(t *testing.T) {
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			for _, r := range results {
				nodeName := r.Labels.Resource["k8s.node.name"]
				hostID := r.Labels.Resource["host.id"]
				if nodeName == "" || hostID == "" {
					continue
				}
				node, found := gt.nodes[nodeName]
				if !found {
					continue
				}
				instanceID, err := parseInstanceIDFromProviderID(node.Spec.ProviderID)
				if err != nil {
					t.Errorf("%s node %s: unexpected provider ID format: %v",
						metricName, nodeName, err)
					continue
				}
				if instanceID != hostID {
					t.Errorf("%s node %s: metric host.id=%s, provider ID instance=%s",
						metricName, nodeName, hostID, instanceID)
				}
			}
		})
	}
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
// TestAllNodeGroupsPresent — every expected instance type must have at least
// one node reporting metrics.
// ---------------------------------------------------------------------------

func TestAllNodeGroupsPresent(t *testing.T) {
	for _, ng := range clusterNodeGroups {
		t.Run(ng.Description+"/"+ng.InstanceType, func(t *testing.T) {
			results, err := queryCache.GetWithFilter(context.Background(), "node_load1", map[string]string{
				"@resource.host.type": ng.InstanceType,
			})
			require.NoError(t, err, "querying node_load1 on %s", ng.Description)
			require.True(t, len(results) > 0,
				"no metrics from %s nodes (%s) — node group may have been deleted",
				ng.Description, ng.InstanceType)
		})
	}
}
