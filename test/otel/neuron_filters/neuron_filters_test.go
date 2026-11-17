//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package neuron_filters validates the otelFilters namespaces, podLabels and
// nodeLabels filters on a Neuron node whose NeuronCores are used by pods in
// different namespaces.
//
// Cluster topology (terraform/eks/daemon/otel-neuron-filters):
//   - 1x inf2.xlarge with 1 Neuron device and 2 NeuronCores, with the test
//     node labels
//   - 1 neuron-burn pod in neuron-keep and 1 in neuron-drop, each with 1 core
//     and the test pod labels
//   - otelFilters.namespaces.exclude: ["neuron-drop"]
//   - otelFilters.podLabels: include "app" and "neuron-filters-keep*",
//     exclude "neuron-filters-keep-secret" and "neuron-filters-keep-drop*"
//   - otelFilters.nodeLabels: include "neuron-filters-node-keep" and
//     "neuron-filters.example.com/*", exclude
//     "neuron-filters.example.com/drop*"
package neuron_filters

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// neuronCoreMetrics are per-core metrics from neuronCoreLevelMetrics in
// test/otel/neuron. Every runtime reports every core of the node, so both
// cores report them while both neuron-burn runtimes run.
var neuronCoreMetrics = []string{"neuroncore_utilization_ratio", "neuroncore_memory_usage_model_shared_scratchpad"}

// burnContainer is the container name of the neuron-burn pods.
const burnContainer = "neuron-burn"

// podKeys are the pod correlation attributes that the namespaces filter
// removes from a core of a filtered pod. awsdevicepodcorrelation sets the
// k8s.* keys, and groupbyattrs/cw_k8s_ci_v0_neuron lifts them to resource
// scope. transform/cw_k8s_ci_v0_device_namespaces also deletes the raw pod,
// namespace and container keys at data point scope. The checks look for every
// key at both scopes.
var podKeys = []string{
	"k8s.pod.name", "k8s.namespace.name", "k8s.container.name",
	"pod", "namespace", "container",
}

// runtimeTagKey is the runtime tag. NEURON_PROCESS_TAG sets it to the pod
// name of the runtime, and each runtime reports every core, so it can carry
// the neuron-drop pod name on both cores. It is not a pod correlation
// attribute, and the namespaces filter keeps it.
const runtimeTagKey = "aws.neuron.runtime.tag"

// Label attributes kept or dropped by the podLabels and nodeLabels filters.
var (
	keptPodLabels = []string{
		// The exact include "app".
		"k8s.pod.label.app",
		// The prefix include "neuron-filters-keep*".
		"k8s.pod.label.neuron-filters-keep-a",
		// The prefix include covers it, and the exact exclude
		// "neuron-filters-keep-secret" must not match it as a prefix.
		"k8s.pod.label.neuron-filters-keep-secret-x",
	}
	droppedPodLabels = []string{
		// The exact include "app" must not match it as a prefix.
		"k8s.pod.label.app-extra",
		// The exact exclude.
		"k8s.pod.label.neuron-filters-keep-secret",
		// Inside the include prefix, and the exclude prefix
		// "neuron-filters-keep-drop*" removes it.
		"k8s.pod.label.neuron-filters-keep-drop-x",
		// podLabels.include does not cover it, so the agent never collects it.
		"k8s.pod.label.neuron-filters-other",
	}
	keptNodeLabels = []string{
		// The exact include "neuron-filters-node-keep".
		"k8s.node.label.neuron-filters-node-keep",
		// The dotted prefix include "neuron-filters.example.com/*".
		"k8s.node.label.neuron-filters.example.com/keep-x",
	}
	droppedNodeLabels = []string{
		// The exact include "neuron-filters-node-keep" must not match it as a
		// prefix.
		"k8s.node.label.neuron-filters-node-keep-extra",
		// nodeLabels.include does not cover them, so the agent never collects
		// them.
		"k8s.node.label.neuron-filters-node-other",
		"k8s.node.label.aws.amazon.com/neuron.present",
		// Inside the dotted include prefix, and the dotted exclude prefix
		// "neuron-filters.example.com/drop*" removes it.
		"k8s.node.label.neuron-filters.example.com/drop-a",
	}
)

const (
	attempts      = 20
	retryInterval = 30 * time.Second
)

// TestNeuronFilters checks each per-core Neuron metric from the Neuron node:
//
//  1. Exactly two distinct cores (aws.neuron.device and aws.neuron.core at
//     data point scope) are exported.
//  2. Exactly one core has, on every series, the neuron-keep pod, namespace
//     and container as k8s.pod.name, k8s.namespace.name and
//     k8s.container.name at resource scope.
//  3. The other core has none of podKeys at resource or data point scope,
//     and no pod label.
//  4. Every series of the neuron-keep core has every kept pod label, and
//     every series of both cores has every kept node label.
//  5. No series has a dropped pod or node label.
//  6. No series carries the neuron-drop namespace, pod name or pod UID as the
//     value of an attribute at resource or data point scope, except the
//     runtime tag (runtimeTagKey).
//
// The namespaces filter never drops device metrics. For a core used by a pod
// in a filtered namespace, the chart keeps the data points and removes the
// pod correlation attributes (transform/cw_k8s_ci_v0_device_namespaces), so
// the core looks like an unused core with node attributes only. The node has
// no unused core, so the stripped core of check 3 is the neuron-drop core.
//
// The checks catch these chart faults:
//
//   - A filter that drops the data points of the neuron-drop pod, as the old
//     namespace filter did: the neuron-drop core is missing, so check 1
//     fails.
//   - transform/cw_k8s_ci_v0_device_namespaces missing from the Neuron
//     pipeline, or placed before groupbyattrs/cw_k8s_ci_v0_neuron lifts the
//     k8s.* keys to resource scope, where its resource statements find no
//     namespace: the neuron-drop core keeps its pod, so check 3 fails, and
//     check 6 fails on the neuron-drop values.
//   - A transform that strips the wrong core, or no core: check 2 or check 3
//     fails.
//   - k8sattributes/cw_k8s_ci_v0_pod or k8sattributes/cw_k8s_ci_v0_node
//     missing from the Neuron pipeline, or an include entry that does not
//     match: a kept label is missing, so check 4 fails.
//   - awsattributelimit/cw_k8s_ci_v0 missing from the Neuron pipeline, an
//     exclude entry that does not match, or an exact include entry that
//     matches as a prefix: a dropped label arrives, so check 5 fails.
func TestNeuronFilters(t *testing.T) {
	for _, metric := range neuronCoreMetrics {
		metric := metric
		t.Run(metric, func(t *testing.T) {
			t.Parallel()
			// Checks 5 and 6 run only after checks 1 to 4 pass, so the series
			// of both cores are known to arrive with their labels.
			for _, r := range settled(t, metric) {
				for _, problem := range leaks(r) {
					t.Errorf("%s: %s", metric, problem)
				}
			}
		})
	}
}

// settled retries until the node's series of metric pass checks 1 to 4, and
// returns them. The Terraform starts the chart only after both neuron-burn
// pods hold their cores, and a core reports only once a runtime runs, so no
// core reports before its pod holds it. If no attempt passes, settled stops
// the test with the last problems.
func settled(t *testing.T, metric string) []otelmetrics.MetricResult {
	t.Helper()
	selector := fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q,"@resource.k8s.node.name"=%q}`,
		metric, cfg.ClusterName, node)
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

// check returns the failures of checks 1 to 4.
func check(results []otelmetrics.MetricResult) []string {
	byCore := map[string][]otelmetrics.MetricResult{}
	for _, r := range results {
		if core(r) == "" {
			return []string{fmt.Sprintf("check 1: series without a core: resource %v, datapoint %v", r.Labels.Resource, r.Labels.Datapoint)}
		}
		byCore[core(r)] = append(byCore[core(r)], r)
	}
	names := make([]string, 0, len(byCore))
	for c := range byCore {
		names = append(names, c)
	}
	sort.Strings(names)
	if len(names) != expectedCoreCount {
		return []string{fmt.Sprintf("check 1: want %d cores, got %d: %q", expectedCoreCount, len(names), names)}
	}

	// kept has the cores whose series all carry the neuron-keep pod.
	// stripped has the cores without any pod key or pod label.
	var kept, stripped []string
	for _, c := range names {
		switch {
		case all(byCore[c], hasKeepPod):
			kept = append(kept, c)
		case all(byCore[c], hasNoPodKeys):
			stripped = append(stripped, c)
		}
	}
	if len(kept) != 1 {
		return []string{fmt.Sprintf("check 2: want 1 core with the neuron-keep pod %q in namespace %q on every series, got %q: %s",
			keepPod, keepNamespace, kept, describe(byCore, names))}
	}
	if len(stripped) != expectedCoreCount-1 {
		return []string{fmt.Sprintf("check 3: want %d cores without %q or a pod label at resource or data point scope, got %q: %s",
			expectedCoreCount-1, podKeys, stripped, describe(byCore, names))}
	}

	var problems []string
	for _, r := range byCore[kept[0]] {
		if missing := missingKeys(r, keptPodLabels); len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("check 4: neuron-keep core %s series without pod labels %q: resource %v", kept[0], missing, r.Labels.Resource))
		}
	}
	for _, c := range names {
		for _, r := range byCore[c] {
			if missing := missingKeys(r, keptNodeLabels); len(missing) > 0 {
				problems = append(problems, fmt.Sprintf("check 4: core %s series without node labels %q: resource %v", c, missing, r.Labels.Resource))
			}
		}
	}
	return problems
}

// leaks returns the failures of checks 5 and 6 for one series.
func leaks(r otelmetrics.MetricResult) []string {
	var problems []string
	for _, scope := range []struct {
		name   string
		labels map[string]string
	}{{"resource", r.Labels.Resource}, {"datapoint", r.Labels.Datapoint}} {
		for _, key := range append(append([]string{}, droppedPodLabels...), droppedNodeLabels...) {
			if _, ok := scope.labels[key]; ok {
				problems = append(problems, fmt.Sprintf("check 5: core %s has the filtered label %s at %s scope", core(r), key, scope.name))
			}
		}
		for k, v := range scope.labels {
			if k != runtimeTagKey && (v == dropNamespace || v == dropPod || v == dropPodUID) {
				problems = append(problems, fmt.Sprintf("check 6: core %s has the filtered pod value %q in %s at %s scope", core(r), v, k, scope.name))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// hasKeepPod reports whether the series carries the neuron-keep pod,
// namespace and container at resource scope.
func hasKeepPod(r otelmetrics.MetricResult) bool {
	return r.Labels.Resource["k8s.pod.name"] == keepPod && r.Labels.Resource["k8s.namespace.name"] == keepNamespace &&
		r.Labels.Resource["k8s.container.name"] == burnContainer
}

// hasNoPodKeys reports whether the series has none of podKeys and no pod
// label at resource or data point scope.
func hasNoPodKeys(r otelmetrics.MetricResult) bool {
	for _, labels := range []map[string]string{r.Labels.Resource, r.Labels.Datapoint} {
		for _, k := range podKeys {
			if _, ok := labels[k]; ok {
				return false
			}
		}
		for k := range labels {
			if strings.HasPrefix(k, "k8s.pod.label.") {
				return false
			}
		}
	}
	return true
}

// missingKeys returns the keys that the series has at neither resource nor
// data point scope.
func missingKeys(r otelmetrics.MetricResult, keys []string) []string {
	var missing []string
	for _, k := range keys {
		_, atResource := r.Labels.Resource[k]
		_, atDatapoint := r.Labels.Datapoint[k]
		if !atResource && !atDatapoint {
			missing = append(missing, k)
		}
	}
	return missing
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
// core.
func describe(byCore map[string][]otelmetrics.MetricResult, names []string) string {
	var parts []string
	for _, c := range names {
		for _, r := range byCore[c] {
			parts = append(parts, fmt.Sprintf("core %s, runtime %q: resource pod %q, namespace %q, container %q; datapoint pod %q, namespace %q", c,
				r.Labels.Resource[runtimeTagKey], r.Labels.Resource["k8s.pod.name"], r.Labels.Resource["k8s.namespace.name"],
				r.Labels.Resource["k8s.container.name"], r.Labels.Datapoint["pod"], r.Labels.Datapoint["namespace"]))
		}
	}
	return strings.Join(parts, "; ")
}

// core returns the series' core as "device/core", from the data point
// attributes that transform/cw_k8s_ci_v0_neuron_hw_attrs sets, or "" if the
// series has no core.
func core(r otelmetrics.MetricResult) string {
	device, hasDevice := r.Labels.Datapoint["aws.neuron.device"]
	c, hasCore := r.Labels.Datapoint["aws.neuron.core"]
	if !hasDevice || !hasCore {
		return ""
	}
	return device + "/" + c
}
