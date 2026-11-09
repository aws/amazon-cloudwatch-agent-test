//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package filters

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The OTLP sender container in every test workload (main.tf, workloads) sends otlpendpoint.MetricName,
// which metricNames.include names, and otlpUnlistedMetric, which it does not, plus a log record and a
// server span, through the cloudwatch-agent Service. The chart's OTLP pipelines apply the same
// otelFilters as Container Insights, so these tests reuse the namespace and label lists above.

const (
	otlpCase           = "filters"
	otlpUnlistedMetric = "otlp_e2e_unlisted"
)

// otlpSpanMetrics are computed from the sender's spans, and metricNames.include does not name them.
var otlpSpanMetrics = []string{"traces.span.metrics.calls", "traces.span.metrics.duration"}

// TestOTLPNamespaceFilter checks that the namespaces filter drops OTLP metrics, logs and spans from
// the dropped namespaces. Absence is checked only after the signal is seen for every kept namespace,
// from every node for metrics and logs.
func TestOTLPNamespaceFilter(t *testing.T) {
	for _, ns := range keptNamespaces {
		seen(t, otlpendpoint.MetricName+" in "+ns, otlpendpoint.MetricName, map[string]string{"@resource.k8s.namespace.name": ns})
	}
	for _, ns := range droppedNamespaces {
		results, err := query(context.Background(), otlpendpoint.MetricName, map[string]string{"@resource.k8s.namespace.name": ns})
		require.NoError(t, err)
		assert.Empty(t, results, "OTLP metric present for filtered namespace %s", ns)
	}

	records := eventually(t, "OTLP logs from included namespaces on every node", func() ([]logRecord, error) {
		records, err := otlpLogs()
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
		assert.Empty(t, byNamespace(records, ns), "OTLP logs present for filtered namespace %s", ns)
	}

	spans := eventually(t, "OTLP spans from included namespaces", func() ([]otlpvalidation.SpanRecord, error) {
		spans, err := otlpSpans()
		if err != nil {
			return nil, err
		}
		for _, ns := range keptNamespaces {
			if len(spansIn(spans, ns)) == 0 {
				return nil, nil
			}
		}
		return spans, nil
	})
	for _, ns := range droppedNamespaces {
		assert.Empty(t, spansIn(spans, ns), "OTLP spans present for filtered namespace %s", ns)
	}
}

// TestOTLPLabelFilter checks the pod and node label filters on OTLP metrics, logs and spans. Each
// signal has its own label processors in the chart's OTLP pipelines.
func TestOTLPLabelFilter(t *testing.T) {
	inKeepA := map[string]string{"@resource.k8s.namespace.name": "filters-keep-a"}

	const metricWhat = "OTLP metric in filters-keep-a"
	results := eventually(t, metricWhat, func() ([]otelmetrics.MetricResult, error) {
		return query(context.Background(), otlpendpoint.MetricName, inKeepA)
	})
	resources := make([]map[string]string, 0, len(results))
	for _, r := range results {
		resources = append(resources, r.Labels.Resource)
	}
	assertLabels(t, metricWhat, resources, keptLabels, droppedLabels)

	const logWhat = "OTLP logs from filters-keep-a"
	records := eventually(t, logWhat, func() ([]logRecord, error) {
		records, err := otlpLogs()
		return byNamespace(records, "filters-keep-a"), err
	})
	resources = resources[:0]
	for _, r := range records {
		resources = append(resources, r.resource())
	}
	assertLabels(t, logWhat, resources, keptLabels, droppedLabels)

	const spanWhat = "OTLP spans from filters-keep-a"
	spans := eventually(t, spanWhat, func() ([]otlpvalidation.SpanRecord, error) {
		spans, err := otlpSpans()
		return spansIn(spans, "filters-keep-a"), err
	})
	resources = resources[:0]
	for _, s := range spans {
		resources = append(resources, stringMap(s.Resource.Attributes))
	}
	assertLabels(t, spanWhat, resources, keptLabels, droppedLabels)
}

// TestOTLPMetricNameFilter checks that a non-empty metricNames.include drops every OTLP metric it does
// not name, span metrics included. Absence is checked only after the named metric is seen from every
// node in an included namespace, so the senders and the pipeline are known to be reporting.
func TestOTLPMetricNameFilter(t *testing.T) {
	const ns = "filters-keep-a"
	inNamespace := map[string]string{"@resource.k8s.namespace.name": ns}
	seen(t, otlpendpoint.MetricName+" in "+ns, otlpendpoint.MetricName, inNamespace)
	for _, metric := range append([]string{otlpUnlistedMetric}, otlpSpanMetrics...) {
		results, err := query(context.Background(), metric, inNamespace)
		require.NoError(t, err)
		assert.Empty(t, results, "filtered OTLP metric %s present in %s", metric, ns)
	}
}

// otlpLogs returns the sender's OTLP log records from the last logLookback.
func otlpLogs() ([]logRecord, error) {
	return logsIn(otlpendpoint.LogGroup(cfg.ClusterName), otlpendpoint.LogBody(otlpCase))
}

// otlpSpans returns the sender's spans of this cluster from Transaction Search, from the last
// logLookback.
func otlpSpans() ([]otlpvalidation.SpanRecord, error) {
	messages, err := queryMessages("aws/spans", fmt.Sprintf(
		"fields @message | filter @message like %q and @message like %q | limit 10000", cfg.ClusterName, otlpendpoint.CaseAttribute))
	if err != nil {
		return nil, err
	}
	var spans []otlpvalidation.SpanRecord
	for _, m := range messages {
		var s otlpvalidation.SpanRecord
		if json.Unmarshal([]byte(m), &s) == nil && fmt.Sprint(s.Attributes[otlpendpoint.CaseAttribute]) == otlpCase {
			spans = append(spans, s)
		}
	}
	return spans, nil
}

func spansIn(spans []otlpvalidation.SpanRecord, ns string) []otlpvalidation.SpanRecord {
	var out []otlpvalidation.SpanRecord
	for _, s := range spans {
		if fmt.Sprint(s.Resource.Attributes["k8s.namespace.name"]) == ns {
			out = append(out, s)
		}
	}
	return out
}

func stringMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}
