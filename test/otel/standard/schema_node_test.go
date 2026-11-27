//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import "github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"

// Rows for the node agent's cadvisor and kubeletstats pipelines, one per series
// scope because each scope carries a different set of resource keys.
func init() {
	labels := []string{"k8s.node.label.", "k8s.pod.label."}
	// The agent DaemonSet runs a pod on every node, so its owner keys are on some series of
	// every pod-level row. The other kinds depend on the cluster's workloads.
	ownerExpected := otelmetrics.ConcatKeys([]string{"k8s.daemonset.name"}, otelmetrics.WorkloadKeys)
	ownerOptional := otelmetrics.Without(otelmetrics.OwnerKindKeys, "k8s.daemonset.name")
	podSchema := func(required []string) otelmetrics.KeySchema {
		return otelmetrics.KeySchema{
			Required:     required,
			Expected:     ownerExpected,
			Optional:     ownerOptional,
			OpenPrefixes: labels,
			Implies:      otelmetrics.WorkloadImplies(),
		}
	}

	cadvisorKeys := otelmetrics.ConcatKeys(otelmetrics.PrometheusReceiverKeys, otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
		"k8s.cluster.name",   // set_cluster_name; protected
		"k8s.namespace.name", // cadvisor_promote; protected
		"k8s.node.name",      // set_node_name, promote_node_name; protected
		"k8s.node.uid",       // k8sattributes/cw_k8s_ci_v0_node
		"k8s.pod.name",       // cadvisor_promote; protected
		"k8s.pod.uid",        // k8sattributes/cw_k8s_ci_v0_pod; protected
	})
	kubeletstatsPodKeys := otelmetrics.ConcatKeys(otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
		"k8s.cluster.name",   // set_cluster_name; protected
		"k8s.namespace.name", // kubeletstats/cw_k8s_ci_v0; protected
		"k8s.node.name",      // k8sattributes/cw_k8s_ci_v0_pod; protected
		"k8s.node.uid",       // k8sattributes/cw_k8s_ci_v0_node
		"k8s.pod.name",       // kubeletstats/cw_k8s_ci_v0; protected
		"k8s.pod.uid",        // kubeletstats/cw_k8s_ci_v0, k8sattributes/cw_k8s_ci_v0_pod; protected
	})

	attributeSchemas = append(attributeSchemas,
		groupSchema{
			name:    "cadvisor-container",
			metrics: func() []string { return metricNamesByScope(cadvisorMetrics, otelmetrics.ScopeContainer) },
			schema: otelmetrics.GroupSchema{
				Resource: podSchema(otelmetrics.ConcatKeys(cadvisorKeys, []string{
					"k8s.container.name", // cadvisor_promote; cadvisor_rollup drops series without it; protected
				})),
				Instrumentation: scopeInstrumentation,
			},
		},
		groupSchema{
			// container_network_* is sandbox-only and arrives without `container`, so
			// cadvisor_promote never sets k8s.container.name.
			name:    "cadvisor-pod",
			metrics: func() []string { return metricNamesByScope(cadvisorMetrics, otelmetrics.ScopePod) },
			schema:  otelmetrics.GroupSchema{Resource: podSchema(cadvisorKeys), Instrumentation: scopeInstrumentation},
		},
		groupSchema{
			name:    "kubeletstats-node",
			metrics: func() []string { return metricNames(kubeletstatsNodeMetrics) },
			schema: otelmetrics.GroupSchema{
				Resource: otelmetrics.KeySchema{
					Required: otelmetrics.ConcatKeys(otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
						"k8s.cluster.name", // set_cluster_name; protected
						"k8s.node.name",    // kubeletstats/cw_k8s_ci_v0; protected
						"k8s.node.uid",     // k8sattributes/cw_k8s_ci_v0_node
					}),
					OpenPrefixes: []string{"k8s.node.label."},
				},
				Instrumentation: scopeInstrumentation,
			},
		},
		groupSchema{
			name:    "kubeletstats-pod",
			metrics: func() []string { return metricNames(kubeletstatsPodMetrics) },
			schema:  otelmetrics.GroupSchema{Resource: podSchema(kubeletstatsPodKeys), Instrumentation: scopeInstrumentation},
		},
		groupSchema{
			name:    "kubeletstats-container",
			metrics: func() []string { return metricNames(kubeletstatsContainerMetrics) },
			schema: otelmetrics.GroupSchema{
				Resource: podSchema(otelmetrics.ConcatKeys(kubeletstatsPodKeys, []string{
					"k8s.container.name", // kubeletstats/cw_k8s_ci_v0; protected
				})),
				Instrumentation: scopeInstrumentation,
			},
		},
	)
}
