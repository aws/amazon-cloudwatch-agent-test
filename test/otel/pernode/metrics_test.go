//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

// Workload + label constants shared by the per-node tests. These must match
// resources/workload.yaml (the ServiceMonitor/PodMonitor target_node relabel and
// the namespace/labels) applied by the otel-pernode terraform harness.
const (
	// workloadNamespace is where the sm-app/pm-app workloads run.
	workloadNamespace = "pernode-demo"

	// targetNodeLabel is the metric label that the ServiceMonitor/PodMonitor
	// relabel_configs stamp from __meta_kubernetes_pod_node_name — i.e. the node
	// of the SCRAPED pod. It is an unprefixed PromQL label, so it lands in
	// MetricResult.Labels.Datapoint.
	targetNodeLabel = "target_node"

	// agentNodeResourceKey is the resource attribute carrying the node of the
	// SCRAPING agent (from ${env:K8S_NODE_NAME} via the prometheuscr pipeline).
	// It is an @resource.* label, so it lands in MetricResult.Labels.Resource.
	agentNodeResourceKey = "k8s.node.name"
)

// perNodeMetrics are metrics emitted by the per-node workload and scraped via
// the ServiceMonitor/PodMonitor path. They carry the target_node relabel, so
// every series can be checked for node-locality. http_requests_total is driven
// by the load generator; go_goroutines is emitted by the client_golang default
// registry without traffic and serves as a traffic-independent fallback.
var perNodeMetrics = []string{
	"http_requests_total",
	"go_goroutines",
}
