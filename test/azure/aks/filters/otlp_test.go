//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package filters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otel/otlpendpoint"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The OTLP sender container in every test workload (main.tf, workloads) sends otlpendpoint.MetricName,
// which metricNames.include names, and otlpUnlistedMetric, which it does not, plus a log record and a
// server span, through the cloudwatch-agent Service. Its otlp_e2e_case is its namespace, so a record
// the agent did not match to a pod, which has no k8s.namespace.name, can still be traced to its sender.
// The chart's OTLP pipelines apply the same otelFilters as Container Insights, so these tests reuse
// the namespace and label lists above.

const (
	// otlpBodyPrefix starts the body of every sender's log record: otlpendpoint.LogBody of its case, the
	// namespace, and every test namespace starts with "filters-".
	otlpBodyPrefix     = "otlp-e2e filters-"
	otlpUnlistedMetric = "otlp_e2e_unlisted"
	senderSelector     = "app=filters-e2e"
)

// otlpSpanMetrics are computed from the sender's spans, and metricNames.include does not name them.
var otlpSpanMetrics = []string{"traces.span.metrics.calls", "traces.span.metrics.duration"}

// TestOTLPNamespaceFilter checks that the namespaces filter drops OTLP metrics, logs and spans from
// the dropped namespaces. Absence is checked only after the signal is seen for every kept namespace,
// from every node for metrics and logs.
func TestOTLPNamespaceFilter(t *testing.T) {
	skipWithoutOTLP(t)
	for _, ns := range keptNamespaces {
		seen(t, otlpendpoint.MetricName+" in "+ns, otlpendpoint.MetricName, map[string]string{"@resource.k8s.namespace.name": ns})
	}
	for _, ns := range droppedNamespaces {
		results, err := query(context.Background(), otlpendpoint.MetricName, map[string]string{
			"@resource.k8s.namespace.name": ns,
			otlpendpoint.CaseAttribute:     ns,
		})
		require.NoError(t, err)
		assert.Empty(t, results, "OTLP metric present for filtered namespace %s", ns)
	}

	records := eventually(t, "OTLP logs from included namespaces on every node", func() ([]logRecord, error) {
		records, err := otlpLogs(time.Now().Add(-logLookback))
		if err != nil {
			return nil, err
		}
		for _, ns := range keptNamespaces {
			if !fromEveryNode(byNamespace(byCase(records, ns), ns)) {
				return nil, nil
			}
		}
		return records, nil
	})
	for _, ns := range droppedNamespaces {
		assert.Empty(t, byNamespace(byCase(records, ns), ns), "OTLP logs present for filtered namespace %s", ns)
	}

	t.Run("spans", func(t *testing.T) {
		otlpendpoint.SkipWithoutTransactionSearch(t)
		spans := eventually(t, "OTLP spans from included namespaces", func() ([]otlpvalidation.SpanRecord, error) {
			spans, err := otlpSpans(time.Now().Add(-logLookback))
			if err != nil {
				return nil, err
			}
			for _, ns := range keptNamespaces {
				if len(spansIn(spansOfCase(spans, ns), ns)) == 0 {
					return nil, nil
				}
			}
			return spans, nil
		})
		for _, ns := range droppedNamespaces {
			assert.Empty(t, spansIn(spansOfCase(spans, ns), ns), "OTLP spans present for filtered namespace %s", ns)
		}
	})
}

// TestOTLPNamespaceFilterUnmatched checks the dropped namespaces' OTLP telemetry without filtering on
// k8s.namespace.name, by the senders' case. The chart's namespace filters drop a record only when it
// has k8s.namespace.name, so a record the agent did not match to a pod passes them: a known fail-open
// of the chart. The agent matches a sender's records once its pod cache has the pod, which is
// otlpendpoint.MatchedSince, so this test tolerates the earlier records and requires that none of a
// dropped namespace's records, matched or not, arrive after it.
//
// This asserts the chart's current behaviour. Whether unmatched records should pass the namespace
// filter is an open decision on aws-observability/helm-charts#432 (was #425). If the chart starts to
// drop unmatched records, the check stays valid and can be tightened to no records at all; if the
// chart instead documents the fail-open as intended, the tolerance stays.
func TestOTLPNamespaceFilterUnmatched(t *testing.T) {
	skipWithoutOTLP(t)
	ctx := context.Background()
	for _, ns := range droppedNamespaces {
		ns := ns
		t.Run(ns, func(t *testing.T) {
			since, err := otlpendpoint.MatchedSince(ctx, kube, ns, senderSelector)
			require.NoError(t, err)

			// An instant query returns a series up to otlpendpoint.Staleness after its last sample, so
			// a series it returns after since+Staleness had a sample after since.
			waitUntil(since.Add(otlpendpoint.Staleness))
			results, err := query(ctx, otlpendpoint.MetricName, map[string]string{otlpendpoint.CaseAttribute: ns})
			require.NoError(t, err)
			for _, r := range results {
				assert.Fail(t, "OTLP metric of a filtered namespace's sender", "%s: %v", ns, r.Labels.Resource)
			}

			// The kept namespaces' records since `since` show that the senders and pipelines report.
			eventually(t, "OTLP logs from a kept namespace since "+since.String(), func() ([]logRecord, error) {
				records, err := otlpLogs(since)
				return byCase(records, keptNamespaces[0]), err
			})
			records, err := otlpLogs(since)
			require.NoError(t, err)
			for _, r := range byCase(records, ns) {
				assert.Fail(t, "OTLP log record of a filtered namespace's sender", "%s: %v", ns, r.resource())
			}

			t.Run("spans", func(t *testing.T) {
				otlpendpoint.SkipWithoutTransactionSearch(t)
				spans, err := otlpSpans(since)
				require.NoError(t, err)
				for _, s := range spansOfCase(spans, ns) {
					assert.Fail(t, "OTLP span of a filtered namespace's sender", "%s: %v", ns, stringMap(s.Resource.Attributes))
				}
			})
		})
	}
}

// TestOTLPLabelFilter checks the pod and node label filters on OTLP metrics, logs and spans. Each
// signal has its own label processors in the chart's OTLP pipelines.
func TestOTLPLabelFilter(t *testing.T) {
	skipWithoutOTLP(t)
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
		records, err := otlpLogs(time.Now().Add(-logLookback))
		return byNamespace(byCase(records, "filters-keep-a"), "filters-keep-a"), err
	})
	resources = resources[:0]
	for _, r := range records {
		resources = append(resources, r.resource())
	}
	assertLabels(t, logWhat, resources, keptLabels, droppedLabels)

	t.Run("spans", func(t *testing.T) {
		otlpendpoint.SkipWithoutTransactionSearch(t)
		const spanWhat = "OTLP spans from filters-keep-a"
		spans := eventually(t, spanWhat, func() ([]otlpvalidation.SpanRecord, error) {
			spans, err := otlpSpans(time.Now().Add(-logLookback))
			return spansIn(spansOfCase(spans, "filters-keep-a"), "filters-keep-a"), err
		})
		var spanResources []map[string]string
		for _, s := range spans {
			spanResources = append(spanResources, stringMap(s.Resource.Attributes))
		}
		assertLabels(t, spanWhat, spanResources, keptLabels, droppedLabels)
	})
}

// TestOTLPMetricNameFilter checks that a non-empty metricNames.include drops every OTLP metric it does
// not name, span metrics included. Absence is checked only after the named metric is seen from every
// node in an included namespace, so the senders and the pipeline are known to be reporting.
func TestOTLPMetricNameFilter(t *testing.T) {
	skipWithoutOTLP(t)
	const ns = "filters-keep-a"
	inNamespace := map[string]string{"@resource.k8s.namespace.name": ns}
	seen(t, otlpendpoint.MetricName+" in "+ns, otlpendpoint.MetricName, inNamespace)
	for _, metric := range append([]string{otlpUnlistedMetric}, otlpSpanMetrics...) {
		results, err := query(context.Background(), metric, inNamespace)
		require.NoError(t, err)
		assert.Empty(t, results, "filtered OTLP metric %s present in %s", metric, ns)
	}
}

// otlpLogs returns the senders' OTLP log records since since, matched to a pod or not.
func otlpLogs(since time.Time) ([]logRecord, error) {
	return logsInSince(otlpendpoint.LogGroup(cfg.ClusterName), otlpBodyPrefix, since)
}

// byCase returns the records of the sender whose case is senderCase.
func byCase(records []logRecord, senderCase string) []logRecord {
	var out []logRecord
	for _, r := range records {
		if r.Body == otlpendpoint.LogBody(senderCase) {
			out = append(out, r)
		}
	}
	return out
}

// otlpSpans returns the senders' spans of this cluster from Transaction Search since since, matched
// to a pod or not: every span the chart exports has k8s.cluster.name, and the senders' spans have the
// case attribute.
func otlpSpans(since time.Time) ([]otlpvalidation.SpanRecord, error) {
	messages, err := queryMessagesSince(otlpendpoint.SpansLogGroup, fmt.Sprintf(
		"fields @message | filter @message like %q and @message like %q | limit 10000", cfg.ClusterName, otlpendpoint.CaseAttribute), since)
	if err != nil {
		return nil, err
	}
	var spans []otlpvalidation.SpanRecord
	for _, m := range messages {
		var s otlpvalidation.SpanRecord
		if json.Unmarshal([]byte(m), &s) == nil && s.Attributes[otlpendpoint.CaseAttribute] != nil {
			spans = append(spans, s)
		}
	}
	return spans, nil
}

// spansOfCase returns the spans of the sender whose case is senderCase.
func spansOfCase(spans []otlpvalidation.SpanRecord, senderCase string) []otlpvalidation.SpanRecord {
	var out []otlpvalidation.SpanRecord
	for _, s := range spans {
		if fmt.Sprint(s.Attributes[otlpendpoint.CaseAttribute]) == senderCase {
			out = append(out, s)
		}
	}
	return out
}

// skipWithoutOTLP skips t when the installed chart has no OTLP endpoint: the suite's Terraform sets
// OTLP_ENDPOINT_IN_CHART=false for a chart without the otlp values block, such as a release before it.
func skipWithoutOTLP(t *testing.T) {
	t.Helper()
	if os.Getenv("OTLP_ENDPOINT_IN_CHART") == "false" {
		t.Skip("the chart has no otlp values block, so it serves no OTLP endpoint")
	}
}

// waitUntil sleeps until t.
func waitUntil(t time.Time) {
	if d := time.Until(t); d > 0 {
		time.Sleep(d)
	}
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
