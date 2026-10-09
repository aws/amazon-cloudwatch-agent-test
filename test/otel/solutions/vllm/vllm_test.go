//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package vllm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The two real vLLM servers and how the chart finds each.
var servers = []struct {
	name       string
	podPattern string
}{
	// KServe model container: serving.kserve.io/inferenceservice and component=predictor labels + container name.
	{name: "kserve", podPattern: inferenceService + "-predictor-.*"},
	// Plain vllm serve: image name.
	{name: "image", podPattern: standalonePrefix + ".*"},
}

func podFilter(pattern string) map[string]string {
	return map[string]string{"~@resource.k8s.pod.name": pattern}
}

// ---------------------------------------------------------------------------
// TestVLLMMetricsExist — every expected metric arrives from both servers.
// ---------------------------------------------------------------------------

func TestVLLMMetricsExist(t *testing.T) {
	t.Parallel()
	for _, srv := range servers {
		srv := srv
		for _, md := range vllmMetrics {
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
// TestVLLMInstrumentation — scope name and pipeline on every series.
// ---------------------------------------------------------------------------

func TestVLLMInstrumentation(t *testing.T) {
	t.Parallel()
	for _, md := range vllmMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), md.Name, map[string]string{"@resource.k8s.namespace.name": namespace})
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available", md.Name)
			for _, r := range results {
				require.Equal(t, scopeVLLM, r.Labels.Instrumentation["@name"], "%s instrumentation name", md.Name)
				require.Equal(t, pipelineVLLM, r.Labels.Instrumentation["cloudwatch.pipeline"], "%s pipeline", md.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestVLLMExpectedLabels — datapoint labels from vLLM survive the pipeline.
// ---------------------------------------------------------------------------

func TestVLLMExpectedLabels(t *testing.T) {
	t.Parallel()
	for _, md := range vllmMetrics {
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
// TestVLLMKServeAttribution — the InferenceService names the service, and the
// workload is the revision's Deployment, not its ReplicaSet.
// ---------------------------------------------------------------------------

func TestVLLMKServeAttribution(t *testing.T) {
	t.Parallel()
	results, err := queryCache.GetWithFilter(context.Background(), "vllm:num_requests_running", podFilter(inferenceService+"-predictor-.*"))
	require.NoError(t, err)
	require.NotEmpty(t, results, "no vLLM series from the InferenceService")
	for _, r := range results {
		res := r.Labels.Resource
		require.Equal(t, inferenceService, res["service.name"], "service.name")
		require.Equal(t, namespace, res["service.namespace"], "service.namespace")
		// The InferenceService arrives as KServe's own pod label; no custom key is added.
		require.Equal(t, inferenceService, res[podLabelISVC], "%s", podLabelISVC)
		requireNoCustomISVCKey(t, r)
		require.Equal(t, "kserve-container", res["k8s.container.name"], "k8s.container.name")
		require.True(t, strings.HasPrefix(res["k8s.workload.name"], inferenceService+"-predictor-") &&
			strings.HasSuffix(res["k8s.workload.name"], "-deployment"),
			"k8s.workload.name %q should be the revision Deployment", res["k8s.workload.name"])
		require.Equal(t, "Deployment", res["k8s.workload.type"], "k8s.workload.type")
		require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
		require.Equal(t, cfg.ClusterName, res["k8s.cluster.name"], "k8s.cluster.name")
		requireCloudIdentity(t, "vllm:num_requests_running", res)
	}
}

// ---------------------------------------------------------------------------
// TestVLLMNoDuplicateSeries — one series per server. The InferenceService has
// enable-prometheus-scraping, so KServe puts prometheus.io/* on the pod; if the
// annotation opt-in applied to KServe pods, the queue-proxy (which proxies the
// model server's /metrics) would be scraped too.
// ---------------------------------------------------------------------------

func TestVLLMNoDuplicateSeries(t *testing.T) {
	t.Parallel()
	for _, srv := range servers {
		srv := srv
		t.Run(srv.name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.GetWithFilter(context.Background(), "vllm:num_requests_running", podFilter(srv.podPattern))
			require.NoError(t, err)
			require.NotEmpty(t, results, "no vllm:num_requests_running from the %s server", srv.name)
			perPod := map[string]int{}
			for _, r := range results {
				require.NotEqual(t, "queue-proxy", r.Labels.Resource["k8s.container.name"], "queue-proxy scraped as a vLLM server")
				perPod[r.Labels.Resource["k8s.pod.name"]]++
			}
			for pod, n := range perPod {
				require.Equal(t, 1, n, "%s has %d vllm:num_requests_running series", pod, n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestVLLMImageDetected — a plain Deployment is named after its workload.
// ---------------------------------------------------------------------------

func TestVLLMImageDetected(t *testing.T) {
	t.Parallel()
	results, err := queryCache.GetWithFilter(context.Background(), "vllm:num_requests_running", podFilter(standalonePrefix+".*"))
	require.NoError(t, err)
	require.NotEmpty(t, results, "no vLLM series from the image-detected server")
	for _, r := range results {
		res := r.Labels.Resource
		require.Equal(t, "vllm-standalone", res["k8s.workload.name"], "k8s.workload.name")
		require.Equal(t, "vllm-standalone", res["service.name"], "service.name")
		require.Equal(t, "ai_inference", res["aws.service.type"], "aws.service.type")
		_, hasISVC := res[podLabelISVC]
		require.False(t, hasISVC, "InferenceService label on a non-KServe pod")
		requireNoCustomISVCKey(t, r)
		requireCloudIdentity(t, "vllm:num_requests_running", res)
	}
}

// ---------------------------------------------------------------------------
// TestVLLMAnnotatedPodIgnored — the chart reads no scrape annotations, so a
// vLLM server from a custom-named image is not scraped despite prometheus.io/*.
// ---------------------------------------------------------------------------

func TestVLLMAnnotatedPodIgnored(t *testing.T) {
	t.Parallel()
	matcher := `,"@resource.k8s.pod.name"=~"` + annotatedPrefix + `.*"` + pipeline(pipelineVLLM)
	for _, name := range []string{"vllm:num_requests_running", "http_requests_total"} {
		require.Empty(t, series(t, name, matcher), "%s from an annotation-only pod reached the vllm pipeline", name)
	}
}

// ---------------------------------------------------------------------------
// TestVLLMGenericExporterIgnored — an annotated non-vLLM exporter contributes
// nothing to the vllm pipeline.
// ---------------------------------------------------------------------------

func TestVLLMGenericExporterIgnored(t *testing.T) {
	t.Parallel()
	matcher := `,"@resource.k8s.pod.name"=~"` + genericPodPattern + `"` + pipeline(pipelineVLLM)
	for _, name := range []string{"http_requests_total", "app_jobs_processed_total"} {
		require.Empty(t, series(t, name, matcher), "%s from a non-vLLM exporter reached the vllm pipeline", name)
	}
}

// ---------------------------------------------------------------------------
// TestVLLMDroppedFamilies — runtime and client-registry series are filtered.
// ---------------------------------------------------------------------------

func TestVLLMDroppedFamilies(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"python_gc_objects_collected_total", "process_resident_memory_bytes", "http_request_size_bytes"} {
		require.Empty(t, series(t, name, pipeline(pipelineVLLM)), "%s should be dropped by the vllm pipeline", name)
	}
}
