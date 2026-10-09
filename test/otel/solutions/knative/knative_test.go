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
	for _, name := range metricNames(controlPlaneMetrics) {
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
	for _, name := range metricNames(controlPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineControlPlane) {
				res := r.Labels.Resource
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				require.Equal(t, "knative-serving", res["k8s.namespace.name"], "k8s.namespace.name")
				deploy := res["k8s.deployment.name"]
				require.True(t, controlPlaneComponents[deploy], "k8s.deployment.name %q is not a Knative component", deploy)
				require.True(t, strings.HasPrefix(res["k8s.pod.name"], deploy+"-"), "k8s.pod.name %q", res["k8s.pod.name"])
				requireCloudIdentity(t, name, res)
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
		require.Empty(t, series(t, name, pipeline(pipelineControlPlane)), "%s should be dropped by the %s pipeline", name, pipelineControlPlane)
	}
}

// ---------------------------------------------------------------------------
// Data plane
// ---------------------------------------------------------------------------

func TestKnativeDataPlaneMetrics(t *testing.T) {
	t.Parallel()
	for _, name := range metricNames(dataPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineDataPlane) {
				require.Equal(t, scopeKnative, r.Labels.Instrumentation["@name"], "%s instrumentation name", name)
			}
		})
	}
}

// The revision pod's identity reaches the series: its Deployment and the
// InferenceService it serves, plus aws.service.type for InferenceService revisions.
func TestKnativeDataPlaneAttribution(t *testing.T) {
	t.Parallel()
	for _, name := range metricNames(dataPlaneMetrics) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, r := range pipelineResults(t, name, pipelineDataPlane) {
				res := r.Labels.Resource
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				require.Equal(t, namespace, res["k8s.namespace.name"], "k8s.namespace.name")
				require.True(t, strings.HasPrefix(res["k8s.pod.name"], inferenceService+"-predictor-"), "k8s.pod.name %q", res["k8s.pod.name"])
				require.True(t, strings.HasPrefix(res["k8s.workload.name"], inferenceService+"-predictor-") &&
					strings.HasSuffix(res["k8s.workload.name"], "-deployment"),
					"k8s.workload.name %q should be the revision Deployment", res["k8s.workload.name"])
				require.Equal(t, inferenceService, res[podLabelISVC], "%s", podLabelISVC)
				requireNoCustomISVCKey(t, r)
				// A revision serving an InferenceService is model inference.
				require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
				requireCloudIdentity(t, name, res)
			}
		})
	}
}
