//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package sglang

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// The two SGLang servers and how the chart finds each.
var servers = []struct {
	name       string
	podPattern string
}{
	// KServe predictor: the shared kserve-model-server job, by KServe's labels.
	{name: "kserve", podPattern: inferenceService + "-predictor-.*"},
	// Plain sglang.launch_server: the sglang job, by image name.
	{name: "image", podPattern: standalonePrefix + ".*"},
}

// recent is the window the routing tests list published metric names over.
const recent = 15 * time.Minute

func podFilter(pattern string) map[string]string {
	return map[string]string{"~@resource.k8s.pod.name": pattern}
}

func pipelineFilter(podPattern, pipeline string) map[string]string {
	return map[string]string{"~@resource.k8s.pod.name": podPattern, otelmetrics.PipelineFilter: pipeline}
}

// ---------------------------------------------------------------------------
// TestSGLangMetricsExist — every expected metric arrives from both servers.
// ---------------------------------------------------------------------------

func TestSGLangMetricsExist(t *testing.T) {
	t.Parallel()
	for _, srv := range servers {
		srv := srv
		for _, md := range sglangMetrics {
			md := md
			t.Run(srv.name+"/"+md.Name, func(t *testing.T) {
				t.Parallel()
				results, err := queryCache.GetWithFilter(context.Background(), md.Name, podFilter(srv.podPattern))
				require.NoError(t, err, "querying %s", md.Name)
				require.NotEmpty(t, results, "%s not available from the %s server", md.Name, srv.name)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TestSGLangInstrumentation — scope name and pipeline on every series.
// ---------------------------------------------------------------------------

func TestSGLangInstrumentation(t *testing.T) {
	t.Parallel()
	for _, md := range sglangMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, map[string]string{"@resource.k8s.namespace.name": namespace})
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, scopeSGLang, r.Labels.Instrumentation["@name"], "%s instrumentation name", md.Name)
				require.Equal(t, pipelineSGLang, r.Labels.Instrumentation["cloudwatch.pipeline"], "%s pipeline", md.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSGLangExpectedLabels — datapoint labels from SGLang survive the pipeline.
// ---------------------------------------------------------------------------

func TestSGLangExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range sglangMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, podFilter(inferenceService+"-predictor-.*"))
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				for _, label := range md.ExpectedLabels {
					_, ok := r.Labels.Datapoint[label]
					require.True(t, ok, "%s missing expected label '%s'", md.Name, label)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSGLangKServeAttribution — a KServe predictor is named after its
// InferenceService, as for vLLM, and the workload is the revision's Deployment.
// ---------------------------------------------------------------------------

func TestSGLangKServeAttribution(t *testing.T) {
	t.Parallel()
	results, err := queryCache.GetWithFilter(context.Background(), "sglang:num_running_reqs", podFilter(inferenceService+"-predictor-.*"))
	require.NoError(t, err)
	require.NotEmpty(t, results, "no SGLang series from the InferenceService")
	for _, r := range results {
		res := r.Labels.Resource
		workload := res["k8s.workload.name"]
		require.True(t, strings.HasPrefix(workload, inferenceService+"-predictor-") && strings.HasSuffix(workload, "-deployment"),
			"k8s.workload.name %q should be the revision Deployment", workload)
		require.Equal(t, inferenceService, res["service.name"], "service.name")
		require.Equal(t, namespace, res["service.namespace"], "service.namespace")
		require.Equal(t, "Deployment", res["k8s.workload.type"], "k8s.workload.type")
		require.Equal(t, inferenceService, res[otelmetrics.PodLabelKServeInferenceService], "%s", otelmetrics.PodLabelKServeInferenceService)
		require.Equal(t, "predictor", res["k8s.pod.label.component"], "k8s.pod.label.component")
		// Set by the agent's Prometheus receiver from the scrape target.
		require.Equal(t, "kserve-container", res["k8s.container.name"], "k8s.container.name")
		require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
		require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
		otelmetrics.RequireCloudIdentity(t, cfg, "sglang:num_running_reqs", res)
		otelmetrics.RequireNoAttribute(t, r, "cw.tmp.service_name_annotation")
		otelmetrics.RequireNoAttribute(t, r, "otel_service_name_annotation")
	}
}

// ---------------------------------------------------------------------------
// TestSGLangImageDetected — a plain Deployment declaring no port is scraped on
// sglang.launch_server's 30000, and its resource.opentelemetry.io/service.name
// annotation wins over the workload name.
// ---------------------------------------------------------------------------

func TestSGLangImageDetected(t *testing.T) {
	t.Parallel()
	results, err := queryCache.GetWithFilter(context.Background(), "sglang:num_running_reqs", podFilter(standalonePrefix+".*"))
	require.NoError(t, err)
	require.NotEmpty(t, results, "no SGLang series from the image-detected server")
	for _, r := range results {
		res := r.Labels.Resource
		require.Equal(t, annotatedServiceName, res["service.name"], "service.name from the annotation")
		require.Equal(t, standaloneWorkload, res["k8s.workload.name"], "k8s.workload.name")
		require.Equal(t, "30000", res["server.port"], "server.port (the default for an undeclared port)")
		require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
		_, hasISVC := res[otelmetrics.PodLabelKServeInferenceService]
		require.False(t, hasISVC, "InferenceService label on a non-KServe pod")
		otelmetrics.RequireNoAttribute(t, r, "otel_service_name_annotation")
		otelmetrics.RequireCloudIdentity(t, cfg, "sglang:num_running_reqs", res)
	}
}

// ---------------------------------------------------------------------------
// TestSGLangNoDuplicateSeries — one series per pod, from the model container.
// ---------------------------------------------------------------------------

func TestSGLangNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	for _, srv := range servers {
		srv := srv
		t.Run(srv.name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), "sglang:num_running_reqs", podFilter(srv.podPattern))
			require.NoError(t, err)
			require.NotEmpty(t, results, "no sglang:num_running_reqs from the %s server", srv.name)
			perPod := map[string]int{}
			for _, r := range results {
				container := r.Labels.Resource["k8s.container.name"]
				require.NotEmpty(t, container, "k8s.container.name")
				require.NotEqual(t, "queue-proxy", container, "queue-proxy scraped as an SGLang server")
				perPod[r.Labels.Resource["k8s.pod.name"]]++
			}
			for pod, n := range perPod {
				require.Equal(t, 1, n, "%s has %d sglang:num_running_reqs series", pod, n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestModelServerRouting — the vLLM and SGLang predictors carry the same KServe
// labels and are scraped by the same job; each engine's series reach only its
// own pipeline. Positive controls: each pod has series in its own pipeline.
// ---------------------------------------------------------------------------

func TestModelServerRouting(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, podPattern, own, other string
	}{
		{"sglang-predictor", inferenceService + "-predictor-.*", pipelineSGLang, pipelineVLLM},
		{"sglang-image", standalonePrefix + ".*", pipelineSGLang, pipelineVLLM},
		{"vllm-predictor", vllmPredictorPrefix + ".*", pipelineVLLM, pipelineSGLang},
		{"vllm-image", vllmStandalonePrefix + ".*", pipelineVLLM, pipelineSGLang},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.NotEmpty(t, otelmetrics.PublishedMetricNames(t, queryCache, pipelineFilter(c.podPattern, c.own), recent),
				"positive control: nothing from %s pods in the %s pipeline", c.podPattern, c.own)
			require.Empty(t, otelmetrics.PublishedMetricNames(t, queryCache, pipelineFilter(c.podPattern, c.other), recent),
				"%s pods reached the %s pipeline", c.podPattern, c.other)
		})
	}
}

// ---------------------------------------------------------------------------
// TestSGLangOnlyEngineFamilies — the sglang pipeline publishes only sglang:*.
// ---------------------------------------------------------------------------

func TestSGLangOnlyEngineFamilies(t *testing.T) {
	t.Parallel()
	names := otelmetrics.PublishedMetricNames(t, queryCache, map[string]string{otelmetrics.PipelineFilter: pipelineSGLang}, recent)
	require.NotEmpty(t, names, "nothing published by the sglang pipeline")
	for _, n := range names {
		require.True(t, strings.HasPrefix(n, "sglang:"), "%s published by the sglang pipeline", n)
	}
}

// ---------------------------------------------------------------------------
// TestSGLangCountersIncrease — the load generator sends requests to both
// servers, so their request counters must move, not just exist.
// ---------------------------------------------------------------------------

func TestSGLangCountersIncrease(t *testing.T) {
	t.Parallel()
	for _, srv := range servers {
		srv := srv
		for _, name := range []string{"sglang:num_requests_total", "sglang:prompt_tokens_total"} {
			name := name
			t.Run(srv.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				inc := otelmetrics.Increase(t, queryCache, name, pipelineFilter(srv.podPattern, pipelineSGLang), "10m")
				require.Greater(t, inc, 0.0, "%s did not increase over 10m on the %s server", name, srv.name)
			})
		}
	}
}
