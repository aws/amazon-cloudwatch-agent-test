//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package knative

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

var controlPlaneComponents = map[string]bool{"activator": true, "autoscaler": true, "controller": true, "webhook": true}

// pipelineResults returns the metric's series from one pipeline only.
func pipelineResults(t *testing.T, name, pl string) []otelmetrics.MetricResult {
	t.Helper()
	results, err := queryCache.Get(context.Background(), name)
	require.NoError(t, err, "querying %s", name)
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if r.Labels.Instrumentation["cloudwatch.pipeline"] == pl {
			out = append(out, r)
		}
	}
	require.NotEmpty(t, out, "%s not available from the %s pipeline", name, pl)
	return out
}

// ---------------------------------------------------------------------------
// Control plane
// ---------------------------------------------------------------------------

func TestKnativeControlPlaneMetrics(t *testing.T) {
	t.Parallel()
	for _, name := range otelmetrics.MetricNames(controlPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineControlPlane) {
				require.Equal(t, scopeKnative, r.Labels.Instrumentation["@name"], "%s instrumentation name", name)
			}
		})
	}
}

// Series describe the component pod, not the cluster-scraper that scraped it.
func TestKnativeControlPlaneAttribution(t *testing.T) {
	t.Parallel()
	for _, name := range otelmetrics.MetricNames(controlPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineControlPlane) {
				res := r.Labels.Resource
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				require.Equal(t, "knative-serving", res["k8s.namespace.name"], "k8s.namespace.name")
				deploy := res["k8s.deployment.name"]
				// The component name arrives as the standard pod label; no custom key.
				require.Equal(t, deploy, res["k8s.pod.label.app"], "k8s.pod.label.app")
				otelmetrics.RequireNoAttribute(t, r, "knative_component")
				require.True(t, controlPlaneComponents[deploy], "k8s.deployment.name %q is not a Knative component", deploy)
				require.True(t, strings.HasPrefix(res["k8s.pod.name"], deploy+"-"), "k8s.pod.name %q", res["k8s.pod.name"])
				otelmetrics.RequireCloudIdentity(t, cfg, name, res)
				for _, attr := range []string{"cloud.availability_zone", "host.name", "host.id"} {
					_, ok := res[attr]
					require.False(t, ok, "%s carries %s from the scraper's node", name, attr)
				}
			}
		})
	}
}

func TestKnativeControlPlaneRuntimeDropped(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"go_goroutines", "go_gc_duration_seconds", "process_cpu_seconds_total"} {
		require.Empty(t, otelmetrics.Series(t, client, cfg.ClusterName, name, otelmetrics.PipelineMatcher(pipelineControlPlane)), "%s should be dropped by the %s pipeline", name, pipelineControlPlane)
	}
}

// ---------------------------------------------------------------------------
// Data plane
// ---------------------------------------------------------------------------

func TestKnativeDataPlaneMetrics(t *testing.T) {
	t.Parallel()
	for _, name := range otelmetrics.MetricNames(dataPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineDataPlane) {
				require.Equal(t, scopeKnative, r.Labels.Instrumentation["@name"], "%s instrumentation name", name)
			}
		})
	}
}

// dataPlaneResultsFor returns the data-plane series of the revision pods whose
// name starts with prefix, requiring at least one.
func dataPlaneResultsFor(t *testing.T, name, prefix string) []otelmetrics.MetricResult {
	t.Helper()
	var out []otelmetrics.MetricResult
	for _, r := range pipelineResults(t, name, pipelineDataPlane) {
		if strings.HasPrefix(r.Labels.Resource["k8s.pod.name"], prefix) {
			out = append(out, r)
		}
	}
	require.NotEmpty(t, out, "no %s series from %s* pods", name, prefix)
	return out
}

// The predictor revision's identity reaches the series: its Deployment and the
// InferenceService it serves, plus aws.service.type = ai_inference.
func TestKnativeDataPlaneAttribution(t *testing.T) {
	t.Parallel()
	for _, name := range otelmetrics.MetricNames(dataPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range dataPlaneResultsFor(t, name, predictorPrefix) {
				res := r.Labels.Resource
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				require.Equal(t, namespace, res["k8s.namespace.name"], "k8s.namespace.name")
				require.True(t, strings.HasPrefix(res["k8s.workload.name"], predictorPrefix) &&
					strings.HasSuffix(res["k8s.workload.name"], "-deployment"),
					"k8s.workload.name %q should be the revision Deployment", res["k8s.workload.name"])
				require.Equal(t, inferenceService, res[otelmetrics.PodLabelKServeInferenceService], "%s", otelmetrics.PodLabelKServeInferenceService)
				require.Equal(t, "predictor", res["k8s.pod.label.component"], "k8s.pod.label.component")
				otelmetrics.RequireNoAttribute(t, r, "inferenceservice")
				// The predictor runs the model: model inference.
				require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
				otelmetrics.RequireCloudIdentity(t, cfg, name, res)
			}
		})
	}
}

// Revisions that don't run the model are scraped, but get no aws.service.type:
// the InferenceService's transformer (same InferenceService label as the
// predictor, component=transformer) and a plain Knative Service.
func TestKnativeDataPlaneNonPredictorRevisions(t *testing.T) {
	t.Parallel()
	for _, rev := range []struct{ name, prefix string }{
		{"transformer", transformerPrefix},
		{"plain-ksvc", plainKsvcPrefix},
	} {
		rev := rev
		for _, name := range otelmetrics.MetricNames(dataPlaneMetrics) {
			name := name
			t.Run(rev.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				for _, r := range dataPlaneResultsFor(t, name, rev.prefix) {
					_, tagged := r.Labels.Resource["aws.service.type"]
					require.False(t, tagged, "%s revision series carry aws.service.type=%q", rev.name, r.Labels.Resource["aws.service.type"])
					otelmetrics.RequireNoAttribute(t, r, "inferenceservice")
				}
			})
		}
	}
}
