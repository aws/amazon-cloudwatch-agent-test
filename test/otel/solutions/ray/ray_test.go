//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ray

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The two Ray pods of the RayService and the node type KubeRay labels each with.
var rayPods = []struct {
	name, podPattern, nodeType string
}{
	{name: "head", podPattern: headPodPattern, nodeType: "head"},
	{name: "worker", podPattern: workerPodPattern, nodeType: "worker"},
}

// recent is the window the routing tests list published metric names over.
const recent = 15 * time.Minute

func podFilter(pattern string) map[string]string {
	return map[string]string{"~@resource.k8s.pod.name": pattern}
}

func pipelineFilter(podPattern, pipeline string) map[string]string {
	return map[string]string{"~@resource.k8s.pod.name": podPattern, otelmetrics.PipelineFilter: pipeline}
}

func namespaceFilter() map[string]string {
	return map[string]string{"@resource.k8s.namespace.name": namespace}
}

// ---------------------------------------------------------------------------
// TestRayNodeMetricsExist — Ray core metrics arrive from the head and the worker.
// ---------------------------------------------------------------------------

func TestRayNodeMetricsExist(t *testing.T) {
	t.Parallel()
	for _, pod := range rayPods {
		pod := pod
		for _, md := range nodeMetrics {
			md := md
			t.Run(pod.name+"/"+md.Name, func(t *testing.T) {
				t.Parallel()
				results, err := queryCache.GetWithFilter(context.Background(), md.Name, podFilter(pod.podPattern))
				require.NoError(t, err, "querying %s", md.Name)
				require.NotEmpty(t, results, "%s not available from the Ray %s", md.Name, pod.name)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TestRayServeMetricsExist — Ray Serve metrics arrive from the RayService.
// ---------------------------------------------------------------------------

func TestRayServeMetricsExist(t *testing.T) {
	t.Parallel()
	for _, md := range serveMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, namespaceFilter())
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
		})
	}
}

// ---------------------------------------------------------------------------
// TestRayInstrumentationAndLabels — scope, pipeline and datapoint labels.
// ---------------------------------------------------------------------------

func TestRayInstrumentationAndLabels(t *testing.T) {
	t.Parallel()
	for _, md := range append(append([]otelmetrics.MetricDefinition{}, nodeMetrics...), serveMetrics...) {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, namespaceFilter())
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, scopeRay, r.Labels.Instrumentation["@name"], "%s instrumentation name", md.Name)
				require.Equal(t, pipelineRay, r.Labels.Instrumentation["cloudwatch.pipeline"], "%s pipeline", md.Name)
				for _, label := range md.ExpectedLabels {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestRayMillisecondUnits — Ray's *_ms series carry unit ms, which the shared
// set_unit rule (_milliseconds) does not match.
// ---------------------------------------------------------------------------

func TestRayMillisecondUnits(t *testing.T) {
	t.Parallel()
	for _, md := range serveMetrics {
		if md.Unit != "ms" {
			continue
		}
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, namespaceFilter())
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, "ms", r.Labels.Datapoint["__unit__"], "%s unit", md.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestRayAttribution — each Ray pod's series carry KubeRay's pod labels, the
// pod as service.name (a RayCluster is not a workload kind), and cloud
// identity.
// ---------------------------------------------------------------------------

func TestRayAttribution(t *testing.T) {
	t.Parallel()
	for _, pod := range rayPods {
		pod := pod
		t.Run(pod.name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), "ray_node_cpu_utilization", podFilter(pod.podPattern))
			require.NoError(t, err)
			require.NotEmpty(t, results, "no ray_node_cpu_utilization from the Ray %s", pod.name)
			for _, r := range results {
				res := r.Labels.Resource
				require.Equal(t, namespace, res["k8s.namespace.name"], "k8s.namespace.name")
				require.Equal(t, namespace, res["service.namespace"], "service.namespace")
				require.Equal(t, res["k8s.pod.name"], res["service.name"], "service.name should be the pod name")
				require.True(t, strings.HasPrefix(res["k8s.pod.label.ray.io/cluster"], rayService+"-"),
					"k8s.pod.label.ray.io/cluster %q should be the RayService's RayCluster", res["k8s.pod.label.ray.io/cluster"])
				require.Equal(t, pod.nodeType, res["k8s.pod.label.ray.io/node-type"], "k8s.pod.label.ray.io/node-type")
				require.Equal(t, "yes", res["k8s.pod.label.ray.io/is-ray-node"], "k8s.pod.label.ray.io/is-ray-node")
				require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
				otelmetrics.RequireCloudIdentity(t, cfg, "ray_node_cpu_utilization", res)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestRayNoDuplicateSeries — one series per pod: pods are scraped, not the two
// Services a RayService puts the head behind.
// ---------------------------------------------------------------------------

func TestRayNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	for _, pod := range rayPods {
		pod := pod
		t.Run(pod.name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), "ray_node_cpu_utilization", podFilter(pod.podPattern))
			require.NoError(t, err)
			require.NotEmpty(t, results, "no ray_node_cpu_utilization from the Ray %s", pod.name)
			perPod := map[string]int{}
			for _, r := range results {
				perPod[r.Labels.Resource["k8s.pod.name"]]++
			}
			for p, n := range perPod {
				require.Equal(t, 1, n, "%s has %d ray_node_cpu_utilization series", p, n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestRayOnlyRayFamilies — the ray pipeline publishes only ray_* names, and Ray
// pods reach no other model-server pipeline. Positive control: the Ray pods'
// series are in the ray pipeline.
// ---------------------------------------------------------------------------

func TestRayOnlyRayFamilies(t *testing.T) {
	t.Parallel()
	names := otelmetrics.PublishedMetricNames(t, queryCache, map[string]string{otelmetrics.PipelineFilter: pipelineRay}, recent)
	require.NotEmpty(t, names, "nothing published by the ray pipeline")
	for _, n := range names {
		require.True(t, strings.HasPrefix(n, "ray_"), "%s published by the ray pipeline", n)
	}
	pods := rayService + "-.*"
	require.NotEmpty(t, otelmetrics.PublishedMetricNames(t, queryCache, pipelineFilter(pods, pipelineRay), recent),
		"positive control: nothing from the Ray pods in the ray pipeline")
	for _, other := range []string{"vllm", "sglang"} {
		require.Empty(t, otelmetrics.PublishedMetricNames(t, queryCache, pipelineFilter(pods, other), recent),
			"Ray pods reached the %s pipeline", other)
	}
}

// ---------------------------------------------------------------------------
// TestRayServeRequestsIncrease — the load generator calls the Serve app, so
// its request counter must move, not just exist.
// ---------------------------------------------------------------------------

func TestRayServeRequestsIncrease(t *testing.T) {
	t.Parallel()
	inc := otelmetrics.Increase(t, queryCache, "ray_serve_num_http_requests_total", map[string]string{
		"@resource.k8s.namespace.name": namespace, otelmetrics.PipelineFilter: pipelineRay,
	}, "10m")
	require.Greater(t, inc, 0.0, "ray_serve_num_http_requests_total did not increase over 10m")
}
