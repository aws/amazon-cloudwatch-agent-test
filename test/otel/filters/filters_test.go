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
var (
	keptNamespaces    = []string{"filters-keep-a", "filters-exact"}
	droppedNamespaces = []string{"filters-keep-secret-b", "filters-other"}
)

// Label attributes kept or dropped by the nodeLabels filter (include
// "filters-e2e*" and "kubernetes.io/os", exclude "filters-e2e-node-drop" and
// "filters-e2e.example.com/*") and the podLabels filter (include "app" and
// "filters-e2e-keep*" and "pod-template-hash", exclude
// "filters-e2e-keep-secret").
var (
	keptLabels = []string{
		"k8s.node.label.filters-e2e-node-keep",
		"k8s.node.label.kubernetes.io/os",
		"k8s.pod.label.app",
		"k8s.pod.label.filters-e2e-keep-a",
	}
	droppedLabels = []string{
		"k8s.node.label.filters-e2e-node-drop",
		"k8s.node.label.filters-e2e.example.com/drop-a",
		"k8s.node.label.filters-e2e.example.com/drop-b",
		"k8s.node.label.kubernetes.io/arch",
		"k8s.pod.label.filters-e2e-keep-secret",
		"k8s.pod.label.filters-e2e-other",
	}
)

// recommendedLabel is included by podLabels and is also a recommended
// exclusion. recommendedExclusions defaults to true for metrics and false for
// logs, so it is dropped from metrics and kept on logs.
const recommendedLabel = "k8s.pod.label.pod-template-hash"

// Metric names kept or dropped by the metricNames filter (include
// "container_*", "kube_pod_info" and "k8s.pod.*", exclude
// "container_network_*" and "container_memory_usage_bytes"). The lists cover
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
	droppedNodeMetrics = []string{
		"node_load1",
		"k8s.node.cpu.usage",
	}
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
	// included namespace.
	for _, metric := range keptMetrics {
		for _, ns := range keptNamespaces {
			eventually(t, metric+" in "+ns, func() ([]otelmetrics.MetricResult, error) {
				return query(context.Background(), metric, map[string]string{"@resource.k8s.namespace.name": ns})
			})
		}
		for _, ns := range droppedNamespaces {
			results, err := query(context.Background(), metric, map[string]string{"@resource.k8s.namespace.name": ns})
			require.NoError(t, err)
			assert.Empty(t, results, "%s present for filtered namespace %s", metric, ns)
		}
	}
}

func TestNamespaceFilterLogs(t *testing.T) {
	records := eventually(t, "application logs from included namespaces", func() ([]logRecord, error) {
		records, err := markerLogs()
		if err != nil {
			return nil, err
		}
		for _, ns := range keptNamespaces {
			if len(byNamespace(records, ns)) == 0 {
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
	results := eventually(t, "container_cpu_usage_seconds_total in filters-keep-a", func() ([]otelmetrics.MetricResult, error) {
		return query(context.Background(), "container_cpu_usage_seconds_total", map[string]string{"@resource.k8s.namespace.name": "filters-keep-a"})
	})
	resources := make([]map[string]string, 0, len(results))
	for _, r := range results {
		resources = append(resources, r.Labels.Resource)
	}
	assertLabels(t, "metrics", resources, keptLabels, concat(droppedLabels, []string{recommendedLabel}))
}

func TestLabelFilterLogs(t *testing.T) {
	records := eventually(t, "application logs from filters-keep-a", func() ([]logRecord, error) {
		records, err := markerLogs()
		return byNamespace(records, "filters-keep-a"), err
	})
	resources := make([]map[string]string, 0, len(records))
	for _, r := range records {
		resources = append(resources, r.resource())
	}
	assertLabels(t, "logs", resources, concat(keptLabels, []string{recommendedLabel}), droppedLabels)
}

func TestMetricNameFilter(t *testing.T) {
	// Each name is queried on its own. Absence is checked only after every
	// kept metric is seen, so the pipelines are known to be reporting.
	const ns = "filters-keep-a"
	inNamespace := map[string]string{"@resource.k8s.namespace.name": ns}
	for _, metric := range keptMetrics {
		eventually(t, metric+" in "+ns, func() ([]otelmetrics.MetricResult, error) {
			return query(context.Background(), metric, inNamespace)
		})
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
func assertLabels(t *testing.T, signal string, resources []map[string]string, kept, dropped []string) {
	t.Helper()
	require.NotEmpty(t, resources, "no %s from filters-keep-a", signal)
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
			assert.NotContains(t, resource, key, "%s has filtered label", signal)
		}
	}
	assert.True(t, hasAll, "no %s resource has all of %v", signal, kept)
}

// concat returns a new slice holding a followed by b.
func concat(a, b []string) []string {
	return append(append(make([]string, 0, len(a)+len(b)), a...), b...)
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

// markerLogs returns the test workloads' log records from the last
// logLookback.
func markerLogs() ([]logRecord, error) {
	logGroup := fmt.Sprintf("/aws/otel/containerinsights/%s/application", cfg.ClusterName)
	end := time.Now()
	rows, err := awsservice.GetLogQueryResults(logGroup, end.Add(-logLookback).Unix(), end.Unix(),
		fmt.Sprintf("fields @message | filter @message like %q | limit 10000", logMarker))
	if err != nil {
		return nil, err
	}
	var records []logRecord
	for _, row := range rows {
		for _, field := range row {
			if aws.ToString(field.Field) != "@message" {
				continue
			}
			var r logRecord
			if json.Unmarshal([]byte(aws.ToString(field.Value)), &r) == nil && strings.HasPrefix(r.Body, logMarker) {
				records = append(records, r)
			}
		}
	}
	return records, nil
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
