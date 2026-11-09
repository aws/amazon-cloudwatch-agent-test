//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package filters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The filters below match terraform/eks/daemon/otel-filters/main.tf.
//
// The absence checks read recent telemetry (the query lookback for metrics,
// logLookback for logs), so they assume a fresh cluster, as in CI. On a
// cluster that ran other filter configs, wait logLookback after the agents
// restart before running these tests.

// Namespaces of the test workloads, kept or dropped by the namespaces filter
// (include "filters-keep-*" and "filters-exact", exclude "filters-keep-secret*").
// filters-exact-other is dropped only if the exact entry "filters-exact" does
// not match as a prefix.
var (
	keptNamespaces    = []string{"filters-keep-a", "filters-exact"}
	droppedNamespaces = []string{"filters-keep-secret-b", "filters-exact-other"}
)

// Label attributes kept or dropped by the nodeLabels filter (include "*" and
// "karpenter.k8s.aws/instance-gpu-name", exclude "filters-e2e-node-drop" and
// "filters-e2e.example.com/*") and the podLabels filter (include "app",
// "filters-e2e-keep*", "filters-e2e.example.com/keep*" and
// "pod-template-hash", exclude "filters-e2e-keep-secret"). Metrics and logs
// share the lists, so both signals keep and drop the same labels.
//
// nodeLabels.include has "*", so the agent collects every node label, and only
// the excludes and the built-in list (recommendedExclusions) remove one.
// podLabels.include has no "*", so the pod labels also cover a label outside
// include, which the agent never collects.
var (
	keptLabels = []string{
		"k8s.node.label.filters-e2e-node-keep",
		// The kubelet sets them on every node. No exclude and no built-in rule
		// removes them, so "*" keeps them.
		"k8s.node.label.kubernetes.io/arch",
		"k8s.node.label.kubernetes.io/os",
		// Inside the built-in prefix "karpenter.k8s.aws/instance-gpu-", but
		// nodeLabels names it in include. The chart then renders
		// transform/cw_k8s_ci_v0_metrics_labels, and the same statements at
		// the start of the logs transform, which keep the named key and delete
		// the rest of the prefix.
		"k8s.node.label.karpenter.k8s.aws/instance-gpu-name",
		"k8s.pod.label.app",
		"k8s.pod.label.filters-e2e-keep-a",
		// The prefix include "filters-e2e-keep*" covers it, and the exact
		// exclude "filters-e2e-keep-secret" must not match it as a prefix.
		"k8s.pod.label.filters-e2e-keep-secret-x",
		// Only the dotted prefix include "filters-e2e.example.com/keep*"
		// covers it, so it arrives only if that prefix matches.
		"k8s.pod.label.filters-e2e.example.com/keep-x",
		// On the built-in list (recommendedExclusions), but podLabels names it
		// in include, and a named include entry wins over the built-in list.
		"k8s.pod.label.pod-template-hash",
	}
	droppedLabels = []string{
		"k8s.node.label.filters-e2e-node-drop",
		"k8s.node.label.filters-e2e.example.com/drop-a",
		"k8s.node.label.filters-e2e.example.com/drop-b",
		// "*" collects it, and the transform deletes it with the rest of the
		// built-in prefix "karpenter.k8s.aws/instance-gpu-". With
		// instance-gpu-name kept, this proves that the transform keeps only
		// the named key.
		"k8s.node.label.karpenter.k8s.aws/instance-gpu-count",
		// The kubelet and the AWS cloud provider set these on every EKS node.
		// They are on the built-in list, so "*" does not keep them. The chart
		// reads the zone for the availability zone before it removes it.
		"k8s.node.label.kubernetes.io/hostname",
		"k8s.node.label.node.kubernetes.io/instance-type",
		"k8s.node.label.topology.kubernetes.io/region",
		"k8s.node.label.topology.kubernetes.io/zone",
		// The exact include "app" must not match it as a prefix.
		"k8s.pod.label.app-extra",
		"k8s.pod.label.filters-e2e-keep-secret",
		// podLabels.include does not cover it, so the agent never collects it.
		"k8s.pod.label.filters-e2e-other",
	}
)

// Metric names kept or dropped by the metricNames filter (include
// "container_*", "kube_pod_info", "k8s.pod.*", "node_load1" and the two
// inferenceMetrics, exclude "container_network_*" and
// "container_memory_usage_bytes"). The lists cover
// both the node agent (cAdvisor, kubelet stats, node exporter) and the cluster
// scraper (kube-state-metrics).
var (
	keptMetrics = []string{
		"container_cpu_usage_seconds_total",
		"kube_pod_info",
		"k8s.pod.cpu.usage",
	}
	// Workload metrics, checked in an included namespace so that only the
	// metricNames filter can explain their absence.
	droppedWorkloadMetrics = []string{
		"container_network_receive_bytes_total",
		"container_memory_usage_bytes",
		"kube_pod_status_phase",
	}
	// Node metrics, which have no namespace, checked cluster-wide.
	// keptNodeMetrics guard the node exporter pipeline; kubelet stats is
	// guarded by k8s.pod.cpu.usage. node_load15 is dropped only if the exact
	// entry "node_load1" does not match as a prefix.
	keptNodeMetrics = []string{
		"node_load1",
	}
	droppedNodeMetrics = []string{
		"node_load15",
		"node_load5",
		"k8s.node.cpu.usage",
	}
)

// inferenceMetrics come from the stand-in inference servers in main.tf
// (inference_stubs), one pod on each node in a kept and in a dropped
// namespace. The first is from the vLLM pipeline and the second from the
// Knative data plane pipeline. Both are in metricNames.include, so only the
// namespaces filter can explain their absence.
var inferenceMetrics = []string{"vllm:num_requests_running", "revision_request_count"}

const (
	inferenceKeptNamespace    = "filters-keep-a"
	inferenceDroppedNamespace = "filters-keep-secret-b"
)

const (
	logMarker     = "otel-filters-e2e"
	logLookback   = 15 * time.Minute
	attempts      = 10
	retryInterval = 30 * time.Second
)

func TestNamespaceFilterMetrics(t *testing.T) {
	// keptMetrics come from the cAdvisor, kube-state-metrics and kubelet stats
	// pipelines, each of which applies the namespaces filter. Terraform waits
	// for every workload to roll out, so the dropped namespaces have running
	// pods. Absence is checked only after the same metric is seen for every
	// included namespace, from every node for the node agent metrics.
	for _, metric := range keptMetrics {
		for _, ns := range keptNamespaces {
			seen(t, metric+" in "+ns, metric, map[string]string{"@resource.k8s.namespace.name": ns})
		}
		for _, ns := range droppedNamespaces {
			results, err := query(context.Background(), metric, map[string]string{"@resource.k8s.namespace.name": ns})
			require.NoError(t, err)
			assert.Empty(t, results, "%s present for filtered namespace %s", metric, ns)
		}
	}
}

// TestNamespaceFilterInferenceMetrics checks the namespaces filter on the
// vLLM and Knative data plane pipelines. Their scrapes take the pod and
// namespace from scrape labels, and transform/cw_k8s_ci_v0_vllm_promote and
// transform/cw_k8s_ci_v0_knative_dataplane_promote copy them to the resource.
// Absence is checked only after each metric is seen for the kept namespace
// from every node, so both pipelines are known to report on every node agent
// that also scrapes a dropped pod.
//
// The checks catch these chart faults:
//
//   - A vLLM or Knative data plane pipeline without
//     filter/cw_k8s_ci_v0_metrics_namespaces: its metric arrives for the
//     dropped namespace.
//   - The filter before the promote transform: k8s.namespace.name is not
//     set yet, so the filter keeps every data point and the metric arrives
//     for the dropped namespace.
//   - A pipeline that is off or that the metricNames filter drops: the
//     metric never arrives for the kept namespace.
func TestNamespaceFilterInferenceMetrics(t *testing.T) {
	kept := map[string]string{"@resource.k8s.namespace.name": inferenceKeptNamespace}
	dropped := map[string]string{"@resource.k8s.namespace.name": inferenceDroppedNamespace}
	for _, metric := range inferenceMetrics {
		seen(t, metric+" in "+inferenceKeptNamespace, metric, kept)
	}
	for _, metric := range inferenceMetrics {
		results, err := query(context.Background(), metric, dropped)
		require.NoError(t, err)
		assert.Empty(t, results, "%s present for filtered namespace %s", metric, inferenceDroppedNamespace)
	}
}

func TestNamespaceFilterLogs(t *testing.T) {
	records := eventually(t, "application logs from included namespaces on every node", func() ([]logRecord, error) {
		records, err := markerLogs("application")
		if err != nil {
			return nil, err
		}
		for _, ns := range keptNamespaces {
			if !fromEveryNode(byNamespace(records, ns)) {
				return nil, nil
			}
		}
		return records, nil
	})
	for _, ns := range droppedNamespaces {
		assert.Empty(t, byNamespace(records, ns), "logs present for filtered namespace %s", ns)
	}
}

func TestLabelFilterMetrics(t *testing.T) {
	// One metric from each pipeline that reports in this cluster: cAdvisor,
	// kubelet stats and node exporter on the node agents, and
	// kube-state-metrics on the cluster scraper. Each pipeline has its own
	// label processors. Node exporter metrics carry node labels only.
	inKeepA := map[string]string{"@resource.k8s.namespace.name": "filters-keep-a"}
	checks := []struct {
		metric        string
		resource      map[string]string
		kept, dropped []string
	}{
		{"container_cpu_usage_seconds_total", inKeepA, keptLabels, droppedLabels},
		{"k8s.pod.cpu.usage", inKeepA, keptLabels, droppedLabels},
		{"kube_pod_info", inKeepA, keptLabels, droppedLabels},
		{"node_load1", nil, nodeLabels(keptLabels), nodeLabels(droppedLabels)},
	}
	for _, c := range checks {
		what := c.metric
		if c.resource != nil {
			what += " in filters-keep-a"
		}
		results := eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
			return query(context.Background(), c.metric, c.resource)
		})
		resources := make([]map[string]string, 0, len(results))
		for _, r := range results {
			resources = append(resources, r.Labels.Resource)
		}
		assertLabels(t, what, resources, c.kept, c.dropped)
	}
}

func TestLabelFilterLogs(t *testing.T) {
	const what = "application logs from filters-keep-a"
	records := eventually(t, what, func() ([]logRecord, error) {
		records, err := markerLogs("application")
		return byNamespace(records, "filters-keep-a"), err
	})
	resources := make([]map[string]string, 0, len(records))
	for _, r := range records {
		resources = append(resources, r.resource())
	}
	assertLabels(t, what, resources, keptLabels, droppedLabels)
}

func TestLabelFilterHostLogs(t *testing.T) {
	// Host logs carry node labels and no pod labels. The host log writer in
	// main.tf adds a marker line to /var/log/messages on every node.
	const what = "host logs from every node"
	records := eventually(t, what, func() ([]logRecord, error) {
		records, err := markerLogs("host")
		if err != nil || !fromEveryNode(records) {
			return nil, err
		}
		return records, nil
	})
	resources := make([]map[string]string, 0, len(records))
	for _, r := range records {
		resources = append(resources, r.resource())
	}
	assertLabels(t, what, resources, nodeLabels(keptLabels), nodeLabels(droppedLabels))
}

func TestMetricNameFilter(t *testing.T) {
	// Each name is queried on its own. Absence is checked only after every
	// kept metric is seen, from every node for the node agent metrics, so the
	// pipelines are known to be reporting.
	const ns = "filters-keep-a"
	inNamespace := map[string]string{"@resource.k8s.namespace.name": ns}
	for _, metric := range keptMetrics {
		seen(t, metric+" in "+ns, metric, inNamespace)
	}
	for _, metric := range keptNodeMetrics {
		seen(t, metric, metric, nil)
	}
	for _, metric := range droppedWorkloadMetrics {
		results, err := query(context.Background(), metric, inNamespace)
		require.NoError(t, err)
		assert.Empty(t, results, "filtered metric %s present in %s", metric, ns)
	}
	for _, metric := range droppedNodeMetrics {
		results, err := query(context.Background(), metric, nil)
		require.NoError(t, err)
		assert.Empty(t, results, "filtered metric %s present", metric)
	}
}

// assertLabels checks that at least one resource has every kept label and
// that no resource has a dropped label.
func assertLabels(t *testing.T, what string, resources []map[string]string, kept, dropped []string) {
	t.Helper()
	require.NotEmpty(t, resources, "no %s", what)
	hasAll := false
	for _, resource := range resources {
		missing := 0
		for _, key := range kept {
			if _, ok := resource[key]; !ok {
				missing++
			}
		}
		if missing == 0 {
			hasAll = true
		}
		for _, key := range dropped {
			assert.NotContains(t, resource, key, "%s has filtered label", what)
		}
	}
	assert.True(t, hasAll, "no %s resource has all of %v", what, kept)
}

// nodeLabels returns the node label attributes of labels.
func nodeLabels(labels []string) []string {
	var out []string
	for _, label := range labels {
		if strings.HasPrefix(label, "k8s.node.label.") {
			out = append(out, label)
		}
	}
	return out
}

// clusterScraperMetrics come from the single cluster scraper. Every other kept
// metric comes from the node agents.
var clusterScraperMetrics = map[string]bool{"kube_pod_info": true}

// seen retries until metric is reported with the resource attributes. A node
// agent metric must be reported from every node, so that an absence check
// cannot pass because one node's agent is not exporting.
func seen(t *testing.T, what, metric string, resource map[string]string) {
	t.Helper()
	if clusterScraperMetrics[metric] {
		eventually(t, what, func() ([]otelmetrics.MetricResult, error) {
			return query(context.Background(), metric, resource)
		})
		return
	}
	for _, node := range nodes {
		onNode := map[string]string{"@resource.k8s.node.name": node}
		for key, value := range resource {
			onNode[key] = value
		}
		eventually(t, what+" on "+node, func() ([]otelmetrics.MetricResult, error) {
			return query(context.Background(), metric, onNode)
		})
	}
}

func query(ctx context.Context, metric string, resource map[string]string) ([]otelmetrics.MetricResult, error) {
	selector := fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q`, metric, cfg.ClusterName)
	for key, value := range resource {
		selector += fmt.Sprintf(`,%q=%q`, key, value)
	}
	return client.Query(ctx, selector+"}")
}

// logRecord is the part of an OTLP log record, as stored in CloudWatch Logs,
// that these tests read.
type logRecord struct {
	Resource struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"resource"`
	Body string `json:"body"`
}

// resource returns the record's resource attributes as strings.
func (r logRecord) resource() map[string]string {
	out := make(map[string]string, len(r.Resource.Attributes))
	for k, v := range r.Resource.Attributes {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// markerLogs returns the marker log records of the test workloads
// ("application") or of the host log writer ("host") from the last
// logLookback.
func markerLogs(kind string) ([]logRecord, error) {
	return logsIn(fmt.Sprintf("/aws/otel/containerinsights/%s/%s", cfg.ClusterName, kind), logMarker)
}

// logsIn returns the log records in logGroup from the last logLookback whose
// body starts with marker.
func logsIn(logGroup, marker string) ([]logRecord, error) {
	messages, err := queryMessages(logGroup, fmt.Sprintf("fields @message | filter @message like %q | limit 10000", marker))
	if err != nil {
		return nil, err
	}
	var records []logRecord
	for _, m := range messages {
		var r logRecord
		if json.Unmarshal([]byte(m), &r) == nil && strings.HasPrefix(r.Body, marker) {
			records = append(records, r)
		}
	}
	return records, nil
}

// queryMessages runs a Logs Insights query over logGroup for the last
// logLookback and returns the @message of each row.
func queryMessages(logGroup, query string) ([]string, error) {
	end := time.Now()
	rows, err := awsservice.GetLogQueryResults(logGroup, end.Add(-logLookback).Unix(), end.Unix(), query)
	if err != nil {
		return nil, err
	}
	var messages []string
	for _, row := range rows {
		for _, field := range row {
			if aws.ToString(field.Field) == "@message" {
				messages = append(messages, aws.ToString(field.Value))
			}
		}
	}
	return messages, nil
}

// fromEveryNode reports whether records include one from every node.
func fromEveryNode(records []logRecord) bool {
	reported := make(map[string]bool, len(nodes))
	for _, r := range records {
		reported[r.resource()["k8s.node.name"]] = true
	}
	for _, node := range nodes {
		if !reported[node] {
			return false
		}
	}
	return true
}

func byNamespace(records []logRecord, ns string) []logRecord {
	var out []logRecord
	for _, r := range records {
		if r.resource()["k8s.namespace.name"] == ns {
			out = append(out, r)
		}
	}
	return out
}

// eventually retries fetch until it returns a non-empty result and returns
// it. If every attempt fails or is empty, it stops the test, so the absence
// checks that follow only run once the pipelines are known to be reporting.
func eventually[T any](t *testing.T, what string, fetch func() ([]T, error)) []T {
	t.Helper()
	var result []T
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		result, err = fetch()
		if err == nil && len(result) > 0 {
			return result
		}
	}
	require.NoError(t, err, "querying %s", what)
	require.NotEmpty(t, result, "no %s after %d attempts", what, attempts)
	return result
}
