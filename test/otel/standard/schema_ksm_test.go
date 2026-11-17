//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// ---------------------------------------------------------------------------
// kube-state-metrics rows of TestAttributeSchema, one per ksm_test.go bucket and
// workload kind. Every key comes from the cluster scraper's kube_state_metrics
// pipeline. The raw labels groupbyattrs lifts (pod, namespace, node, uid, ...)
// are deleted from the resource after ksm_promote copies them back, so no row
// allows them. EKS only: this suite does not run on AKS.
// ---------------------------------------------------------------------------

// On every KSM series.
var ksmCommonKeys = otelmetrics.ConcatKeys(otelmetrics.PrometheusReceiverKeys,
	otelmetrics.Without(otelmetrics.CloudKeys, ksmZoneKey), []string{
		"k8s.cluster.name", // set_cluster_name; protected
	})

// The zone of the node a series names: nodemetadataenricher sets it with host.id, and
// transform/cw_k8s_ci_v0_ksm_zone takes it from the node's zone label otherwise. A pod's
// zone label replaces it (transform/cw_k8s_ci_v0_pod_topology). A series that names no
// node has none. TestKSMAvailabilityZoneIsTheObjectsZone checks the value.
const ksmZoneKey = "cloud.availability_zone"

// A series whose node k8sattributes knows has the node's labels, so it carries the AZ.
var ksmZoneImplies = map[string][]string{"k8s.node.uid": {ksmZoneKey}}

// ksm_promote from namespace, pod and uid; protected.
var ksmPodKeys = []string{"k8s.namespace.name", "k8s.pod.name", "k8s.pod.uid"}

// nodemetadataenricher sets the host keys and the AZ together, only while the node's
// Lease is fresh, that is while its DaemonSet agent runs, so some series carry them.
// TestKSM_NodeBucket_HasHostAttributes requires them.
var ksmHostImplies = otelmetrics.MergeImplies(otelmetrics.Together(otelmetrics.HostKeys...),
	map[string][]string{"host.id": {ksmZoneKey}})

// The chart installs a DaemonSet (the agent) and Deployments (the operator and the
// cluster scraper), so these owner keys are on some pod series of any cluster.
// Jobs, CronJobs and StatefulSets depend on the cluster's workloads.
var ksmOwnerExpected = otelmetrics.ConcatKeys(
	[]string{"k8s.daemonset.name", "k8s.deployment.name", "k8s.replicaset.name"}, otelmetrics.WorkloadKeys)
var ksmOwnerOptional = []string{"k8s.cronjob.name", "k8s.job.name", "k8s.statefulset.name"}

// One row per workload kind rather than one with the object-name keys optional,
// so a series that loses its own k8s.<kind>.name or gains another kind's fails.
func ksmWorkloadSchema(name string, metrics []string, objectKey string) groupSchema {
	return groupSchema{
		name:    name,
		metrics: func() []string { return metrics },
		schema: otelmetrics.GroupSchema{
			Resource: otelmetrics.KeySchema{
				Required: otelmetrics.ConcatKeys(ksmCommonKeys, []string{
					"k8s.namespace.name", // ksm_promote; protected
					objectKey,            // ksm_promote; protected
					"k8s.workload.name",  // set_workload; protected
					"k8s.workload.type",  // set_workload; protected
				}),
			},
			Instrumentation: scopeInstrumentation,
		},
	}
}

func init() {
	attributeSchemas = append(attributeSchemas,
		groupSchema{
			name:    "ksm-node",
			metrics: func() []string { return ksmNodeBucket },
			schema: otelmetrics.GroupSchema{
				Resource: otelmetrics.KeySchema{
					Required: otelmetrics.ConcatKeys(ksmCommonKeys, []string{
						"k8s.node.name", // ksm_promote; protected
						"k8s.node.uid",  // k8sattributes/cw_k8s_ci_v0_node, set by its node-label watch
					}),
					Expected:     append([]string{ksmZoneKey}, otelmetrics.HostKeys...),
					OpenPrefixes: []string{"k8s.node.label."},
					Implies:      otelmetrics.MergeImplies(ksmHostImplies, ksmZoneImplies),
				},
				Instrumentation: scopeInstrumentation,
			},
		},
		groupSchema{
			name:    "ksm-pod",
			metrics: func() []string { return ksmPodBucket },
			schema: otelmetrics.GroupSchema{
				Resource: otelmetrics.KeySchema{
					Required: otelmetrics.ConcatKeys(ksmCommonKeys, ksmPodKeys),
					// k8s.node.name from ksm_promote or k8sattributes/cw_k8s_ci_v0_pod, then k8s.node.uid
					// from k8sattributes/cw_k8s_ci_v0_node; both only on scheduled pods.
					Expected:     otelmetrics.ConcatKeys([]string{"k8s.node.name", "k8s.node.uid", ksmZoneKey}, otelmetrics.HostKeys, ksmOwnerExpected),
					Optional:     ksmOwnerOptional,
					OpenPrefixes: []string{"k8s.node.label.", "k8s.pod.label."},
					Implies: otelmetrics.MergeImplies(ksmHostImplies, ksmZoneImplies, otelmetrics.WorkloadImplies(),
						otelmetrics.Together("k8s.node.name", "k8s.node.uid")),
				},
				Instrumentation: scopeInstrumentation,
			},
		},
		groupSchema{
			name:    "ksm-container",
			metrics: func() []string { return ksmContainerBucket },
			schema: otelmetrics.GroupSchema{
				Resource: otelmetrics.KeySchema{
					Required: otelmetrics.ConcatKeys(ksmCommonKeys, ksmPodKeys, []string{
						"k8s.container.name", // ksm_promote; protected
						"k8s.node.name",      // k8sattributes/cw_k8s_ci_v0_pod; container status implies a bound pod
						"k8s.node.uid",       // k8sattributes/cw_k8s_ci_v0_node
					}),
					Expected:     otelmetrics.ConcatKeys([]string{ksmZoneKey}, otelmetrics.HostKeys, ksmOwnerExpected),
					Optional:     ksmOwnerOptional,
					OpenPrefixes: []string{"k8s.node.label.", "k8s.pod.label."},
					Implies:      otelmetrics.MergeImplies(ksmHostImplies, ksmZoneImplies, otelmetrics.WorkloadImplies()),
				},
				Instrumentation: scopeInstrumentation,
			},
		},
		ksmWorkloadSchema("ksm-deployment", ksmWorkloadBucket.Deployment, "k8s.deployment.name"),
		ksmWorkloadSchema("ksm-daemonset", ksmWorkloadBucket.DaemonSet, "k8s.daemonset.name"),
		ksmWorkloadSchema("ksm-statefulset", ksmWorkloadBucket.StatefulSet, "k8s.statefulset.name"),
		ksmWorkloadSchema("ksm-job", ksmWorkloadBucket.Job, "k8s.job.name"),
		ksmWorkloadSchema("ksm-cronjob", ksmWorkloadBucket.CronJob, "k8s.cronjob.name"),
		groupSchema{
			name:    "ksm-cluster",
			metrics: func() []string { return ksmClusterBucket },
			schema: otelmetrics.GroupSchema{
				Resource: otelmetrics.KeySchema{
					Required: otelmetrics.ConcatKeys(ksmCommonKeys, []string{
						"k8s.namespace.name", // ksm_promote; protected
					}),
				},
				Instrumentation: scopeInstrumentation,
			},
		},
	)
}
