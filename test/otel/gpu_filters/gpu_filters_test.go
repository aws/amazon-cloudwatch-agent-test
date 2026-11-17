//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package gpu_filters validates the otelFilters namespaces filter on a node
// whose GPUs are used by pods in different namespaces, and that the
// metricNames filter matches DCGM metric names with their case.
//
// Cluster topology (terraform/eks/daemon/otel-gpu-filters):
//   - 1x g4dn.12xlarge with 4 GPUs
//   - Node label: ci-test.example.com/multi-gpu=true
//   - 1 gpu-burn pod in gpu-keep and 1 in gpu-drop, each with 1 GPU
//   - 2 GPUs that no pod uses
//   - otelFilters.namespaces.exclude: ["gpu-drop"]
//   - otelFilters.metricNames.exclude: ["DCGM_FI_DEV_SM_CLOCK", "dcgm_fi_dev_gpu_temp"]
package gpu_filters

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// dcgmMetrics are DCGM metrics from dcgmMetricNamesList in test/otel/gpu
// that every GPU reports, with or without a pod. The metricNames exclude has
// "dcgm_fi_dev_gpu_temp", which must not match DCGM_FI_DEV_GPU_TEMP.
var dcgmMetrics = []string{"DCGM_FI_DEV_GPU_UTIL", "DCGM_FI_DEV_FB_USED", "DCGM_FI_DEV_GPU_TEMP"}

// podKeys are the pod correlation attributes that the namespaces filter
// removes from a GPU of a filtered pod. The k8s.* keys are at resource scope.
// The raw pod, namespace and container keys are the copies at data point
// scope that transform/cw_k8s_ci_v0_dcgm_promote makes. The checks look for
// every key at both scopes.
var podKeys = []string{
	"k8s.pod.name", "k8s.namespace.name", "k8s.container.name",
	"pod", "namespace", "container",
}

const (
	attempts      = 20
	retryInterval = 30 * time.Second
)

// TestGPUNamespaceFilter checks each DCGM metric from the multi-GPU node:
//
//  1. Exactly four distinct GPU UUID values are exported.
//  2. Exactly one GPU has, on every series, the gpu-keep pod and namespace
//     as k8s.pod.name and k8s.namespace.name at resource scope, and as pod
//     and namespace at data point scope.
//  3. The other three GPUs have none of podKeys at resource or data point
//     scope.
//  4. No series carries the gpu-drop namespace, pod name or pod UID as the
//     value of an attribute at resource or data point scope.
//
// The namespaces filter never drops device metrics. For a GPU used by a pod
// in a filtered namespace, the chart keeps the data points and removes the
// pod correlation attributes (transform/cw_k8s_ci_v0_device_namespaces), so
// the GPU looks like an unused GPU with node attributes only. The device
// plugin gives each pod its own GPU, so the gpu-drop GPU is one of the three
// GPUs in check 3, with the two unused GPUs. The metrics cannot tell which of
// the three it is, and check 3 does not need to know.
//
// The checks catch these chart faults:
//
//   - A filter that drops the data points of the gpu-drop pod, as the old
//     namespace filter did: the gpu-drop GPU is missing, so check 1 fails.
//   - A transform that cleans only the resource:
//     transform/cw_k8s_ci_v0_dcgm_promote copies pod, namespace and container
//     onto every DCGM data point, so the gpu-drop GPU keeps its raw copies.
//     Only two GPUs are without pod keys, so check 3 fails, and check 4 fails
//     on the gpu-drop values.
//   - A transform that strips the wrong GPU, or no GPU: check 2 or check 3
//     fails.
func TestGPUNamespaceFilter(t *testing.T) {
	for _, metric := range dcgmMetrics {
		metric := metric
		t.Run(metric, func(t *testing.T) {
			t.Parallel()
			// Check 4 runs only after checks 1 to 3 pass, so the series of
			// all four GPUs are known to arrive.
			for _, r := range settled(t, metric) {
				for scope, labels := range map[string]map[string]string{"resource": r.Labels.Resource, "datapoint": r.Labels.Datapoint} {
					for k, v := range labels {
						assert.False(t, v == dropNamespace || v == dropPod || v == dropPodUID,
							"%s GPU %q has the filtered pod value %q in %s at %s scope", metric, uuid(r), v, k, scope)
					}
				}
			}
		})
	}
}

// settled retries until the node's series of metric pass checks 1 to 3, and
// returns them. If DCGM reports a GPU before its pod holds it, the series
// carry no pod, and they stay in the query lookback for a few minutes. Such
// series look the same as the stripped gpu-drop GPU, so with a filter that
// drops the gpu-drop data points they could fill in for the missing GPU. The
// Terraform starts the chart only after both gpu-burn pods hold their GPUs,
// so such series can only come from the agent start, for all GPUs in the
// same scrape, and they leave the lookback together. So a chart that drops
// the gpu-drop GPU never passes: while the pod-less series are in the
// lookback, the gpu-keep GPU fails check 2, and after they leave, check 1
// fails. If no attempt passes, settled stops the test with the last problems.
func settled(t *testing.T, metric string) []otelmetrics.MetricResult {
	t.Helper()
	selector := nodeSelector(metric)
	var problems []string
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		results, err := client.QueryComplete(context.Background(), selector)
		if err != nil {
			problems = []string{err.Error()}
			continue
		}
		problems = check(results)
		if len(problems) == 0 {
			return results
		}
	}
	t.Fatalf("%s from node %s after %d attempts:\n%s", metric, node, attempts, strings.Join(problems, "\n"))
	return nil
}

// TestMetricNameFilterCase checks that otelFilters.metricNames.exclude drops
// DCGM_FI_DEV_SM_CLOCK and that its entry "dcgm_fi_dev_gpu_temp" does not drop
// DCGM_FI_DEV_GPU_TEMP, because metric names are case-sensitive. The DCGM
// exporter reports both metrics for every GPU. The absence check runs only
// after DCGM_FI_DEV_GPU_TEMP arrives from all four GPUs, so the DCGM pipeline
// is known to export.
//
// The checks catch these chart faults:
//
//   - A name match that ignores case: the lowercase entry drops
//     DCGM_FI_DEV_GPU_TEMP, so the presence check fails.
//   - A DCGM pipeline without filter/cw_k8s_ci_v0_metric_names, or an exclude
//     that the chart does not render: DCGM_FI_DEV_SM_CLOCK arrives.
func TestMetricNameFilterCase(t *testing.T) {
	t.Parallel()
	const kept, dropped = "DCGM_FI_DEV_GPU_TEMP", "DCGM_FI_DEV_SM_CLOCK"
	var gpus map[string]bool
	var err error
	for i := 0; i < attempts && len(gpus) != expectedGPUCount; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		var results []otelmetrics.MetricResult
		results, err = client.QueryComplete(context.Background(), nodeSelector(kept))
		gpus = map[string]bool{}
		for _, r := range results {
			if g := uuid(r); g != "" {
				gpus[g] = true
			}
		}
	}
	require.NoError(t, err, "querying %s", kept)
	require.Len(t, gpus, expectedGPUCount, "%s GPUs from node %s after %d attempts", kept, node, attempts)

	results, err := client.QueryComplete(context.Background(),
		fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q}`, dropped, cfg.ClusterName))
	require.NoError(t, err, "querying %s", dropped)
	assert.Empty(t, results, "%s present, but otelFilters.metricNames.exclude has it", dropped)
}

// nodeSelector selects metric from the multi-GPU node.
func nodeSelector(metric string) string {
	return fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q,"@resource.k8s.node.name"=%q}`,
		metric, cfg.ClusterName, node)
}

// check returns the failures of checks 1 to 3.
func check(results []otelmetrics.MetricResult) []string {
	byGPU := map[string][]otelmetrics.MetricResult{}
	for _, r := range results {
		if uuid(r) == "" {
			return []string{fmt.Sprintf("check 1: series without a UUID: resource %v, datapoint %v", r.Labels.Resource, r.Labels.Datapoint)}
		}
		byGPU[uuid(r)] = append(byGPU[uuid(r)], r)
	}
	names := make([]string, 0, len(byGPU))
	for g := range byGPU {
		names = append(names, g)
	}
	sort.Strings(names)
	if len(names) != expectedGPUCount {
		return []string{fmt.Sprintf("check 1: want %d UUID values, got %d: %q", expectedGPUCount, len(names), names)}
	}

	// kept has the GPUs whose series all carry the gpu-keep pod and
	// namespace. stripped has the GPUs without any pod key.
	var kept, stripped []string
	for _, g := range names {
		switch {
		case all(byGPU[g], hasKeepPod):
			kept = append(kept, g)
		case all(byGPU[g], hasNoPodKeys):
			stripped = append(stripped, g)
		}
	}
	if len(kept) != 1 {
		return []string{fmt.Sprintf("check 2: want 1 GPU with the gpu-keep pod %q in namespace %q on every series, got %q: %s",
			keepPod, keepNamespace, kept, describe(byGPU, names))}
	}
	if len(stripped) != expectedGPUCount-1 {
		return []string{fmt.Sprintf("check 3: want %d GPUs without %q at resource or data point scope, got %q: %s",
			expectedGPUCount-1, podKeys, stripped, describe(byGPU, names))}
	}
	return nil
}

// hasKeepPod reports whether the series carries the gpu-keep pod and
// namespace at resource scope and as the raw copies at data point scope.
func hasKeepPod(r otelmetrics.MetricResult) bool {
	return r.Labels.Resource["k8s.pod.name"] == keepPod && r.Labels.Resource["k8s.namespace.name"] == keepNamespace &&
		r.Labels.Datapoint["pod"] == keepPod && r.Labels.Datapoint["namespace"] == keepNamespace
}

// hasNoPodKeys reports whether the series has none of podKeys at resource or
// datapoint scope.
func hasNoPodKeys(r otelmetrics.MetricResult) bool {
	for _, k := range podKeys {
		if _, ok := r.Labels.Resource[k]; ok {
			return false
		}
		if _, ok := r.Labels.Datapoint[k]; ok {
			return false
		}
	}
	return true
}

func all(results []otelmetrics.MetricResult, pred func(otelmetrics.MetricResult) bool) bool {
	for _, r := range results {
		if !pred(r) {
			return false
		}
	}
	return true
}

// describe lists the pod and namespace of each series at both scopes, per
// GPU.
func describe(byGPU map[string][]otelmetrics.MetricResult, names []string) string {
	var parts []string
	for _, g := range names {
		for _, r := range byGPU[g] {
			parts = append(parts, fmt.Sprintf("GPU %q: resource pod %q, namespace %q; datapoint pod %q, namespace %q", g,
				r.Labels.Resource["k8s.pod.name"], r.Labels.Resource["k8s.namespace.name"],
				r.Labels.Datapoint["pod"], r.Labels.Datapoint["namespace"]))
		}
	}
	return strings.Join(parts, "; ")
}

// uuid returns the series' GPU UUID, from data point or resource scope.
func uuid(r otelmetrics.MetricResult) string {
	if v := r.Labels.Datapoint["UUID"]; v != "" {
		return v
	}
	return r.Labels.Resource["UUID"]
}
