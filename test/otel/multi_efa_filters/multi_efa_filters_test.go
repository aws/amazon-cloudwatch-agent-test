//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package multi_efa_filters validates the otelFilters namespaces filter on a
// node whose two EFA cards are used by pods in different namespaces, and on a
// node with one card used by a pod in a filtered namespace and one free card.
//
// Cluster topology (terraform/eks/daemon/otel-multi-efa-filters):
//   - 2x c6in.32xlarge, each with 2 EFA interfaces (1 per network card)
//   - Node 1, label ci-test.example.com/multi-efa-sm=true: 1 efaburn pod in
//     efa-keep and 1 in efa-drop, each with 1 EFA card
//   - Node 2, label ci-test.example.com/multi-efa-free=true: 1 efaburn-free
//     pod in efa-drop with 1 EFA card, and 1 free card
//   - otelFilters.namespaces.exclude: ["efa-drop"]
package multi_efa_filters

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// efaMetrics are the EFA metrics that test/otel/multi_efa expects per device.
var efaMetrics = []string{"efa_rx_bytes", "efa_tx_bytes", "efa_rx_dropped", "efa_rdma_read_bytes"}

// podKeys are the pod correlation attributes that the namespaces filter
// removes from a device of a filtered pod.
var podKeys = []string{"k8s.pod.name", "k8s.namespace.name", "k8s.container.name"}

const (
	attempts      = 20
	retryInterval = 30 * time.Second
	// staleWindow is longer than the query lookback, the time that a series
	// stays in query results after its last data point.
	staleWindow = 6 * time.Minute
)

// TestEFANamespaceFilter checks each EFA metric from node 1:
//
//  1. Exactly two distinct aws.efa.device values are exported.
//  2. The series of one device carry the efa-keep pod and namespace at
//     resource scope.
//  3. The other device has no k8s.pod.name, k8s.namespace.name or
//     k8s.container.name at resource or data point scope.
//  4. No series carries the efa-drop namespace, pod name or pod UID as the
//     value of an attribute at resource or data point scope.
//
// The namespaces filter never drops device metrics. For a device used by a
// pod in a filtered namespace, the chart keeps the data points and removes
// the pod correlation attributes (transform/cw_k8s_ci_v0_device_namespaces),
// so the device looks like an unused device with node attributes only. The
// chart groups the EFA data points by pod (groupbyattrs/cw_k8s_ci_v0_efa)
// before that transform. Each card then has its own resource with its own pod
// and namespace, and the transform strips only the efa-drop card.
//
// The old transform/cw_k8s_ci_v0_efa_promote copied each card's pod onto the
// node's one shared resource, so the last card won for both cards. The checks
// catch it in either card order:
//
//   - If the efa-drop card comes last, both cards carry efa-drop, and the
//     transform strips both. No device carries the efa-keep pod, so check 2
//     fails.
//   - If the efa-keep card comes last, both cards carry the efa-keep pod. No
//     device is without pod attributes, so check 3 fails.
//
// Node 1 is the check for per-card grouping. TestEFAFreeCardNamespaceFilter
// covers a free card next to a card of a filtered pod, on node 2.
func TestEFANamespaceFilter(t *testing.T) {
	for _, metric := range efaMetrics {
		metric := metric
		t.Run(metric, func(t *testing.T) {
			t.Parallel()
			// Check 4 runs only after checks 1 to 3 pass, so the series of
			// both devices are known to arrive.
			noDropValues(t, metric, settled(t, metric, node, check, nil))
		})
	}
}

// TestEFAFreeCardNamespaceFilter checks each EFA metric from node 2:
//
//  1. Exactly two distinct aws.efa.device values are exported.
//  2. Neither device has k8s.pod.name, k8s.namespace.name or
//     k8s.container.name at resource or data point scope.
//  3. No series carries an efa-drop namespace, pod name or pod UID as the
//     value of an attribute at resource or data point scope.
//
// The efaburn-free card and the free card must both look like unused cards.
// The checks catch these chart faults:
//
//   - The old drop filter with groupbyattrs/cw_k8s_ci_v0_efa: the filter
//     drops the efaburn-free card, so only the free card is left, and
//     check 1 fails with 1 device.
//   - The old drop filter with the old transform/cw_k8s_ci_v0_efa_promote:
//     the promote copies the efaburn-free pod onto the node's one shared
//     resource, so the filter drops both cards, and check 1 fails with no
//     device.
//   - A transform that does not strip the efaburn-free card: check 2 and
//     check 3 fail.
//
// The old transform/cw_k8s_ci_v0_efa_promote with the new device transform
// gives the same result as the correct chart on node 2: the shared resource
// carries the efaburn-free pod, and the transform strips it, so both cards
// are without a pod. Node 1 remains the check for per-card grouping.
func TestEFAFreeCardNamespaceFilter(t *testing.T) {
	for _, metric := range efaMetrics {
		metric := metric
		t.Run(metric, func(t *testing.T) {
			t.Parallel()
			// Check 3 runs only after checks 1 and 2 pass, so the series of
			// both devices are known to arrive.
			noDropValues(t, metric, settled(t, metric, freeNode, checkFree, freeNodeReady))
		})
	}
}

// noDropValues fails if a series carries an efa-drop namespace, pod name or
// pod UID as the value of an attribute at resource or data point scope.
func noDropValues(t *testing.T, metric string, results []otelmetrics.MetricResult) {
	t.Helper()
	for _, r := range results {
		for scope, labels := range map[string]map[string]string{"resource": r.Labels.Resource, "datapoint": r.Labels.Datapoint} {
			for k, v := range labels {
				assert.False(t, isDropValue(v),
					"%s device %q has the filtered pod value %q in %s at %s scope", metric, device(r), v, k, scope)
			}
		}
	}
}

// isDropValue reports whether v is the efa-drop namespace, or the name or
// UID of an efa-drop pod.
func isDropValue(v string) bool {
	return v == dropNamespace || v == dropPod || v == dropPodUID || v == freePod || v == freePodUID
}

// settled retries until the node's series of metric pass checkFn, and returns
// them. If ready is not nil, an attempt runs checkFn only after ready returns
// no problems.
//
// On node 1, checkFn is check. If the agent scrapes a card before it knows
// the card's pod, the series carry no pod, and they stay in the query
// lookback for a few minutes. Such series look the same as the stripped
// efa-drop card. The Terraform starts the first agent only after all efaburn
// pods hold their cards, so such series can only come from the agent start,
// for both cards of the node in the same scrape, and they leave the lookback
// together. So a chart that
// drops the efa-drop card never passes: while the pod-less series are in the
// lookback, the efa-keep card fails check 2, and after they leave, check 1
// fails. The old transform/cw_k8s_ci_v0_efa_promote never passes either,
// because both cards carry the same pod, or neither carries one.
//
// On node 2, checkFn is checkFree, and ready is freeNodeReady. Node 2 has no
// kept card that pod-less series could fail, so with the old drop filter the
// pod-less series of the efaburn-free card could fill in for the dropped
// card. freeNodeReady waits until they left the lookback.
//
// The selector has the node name, so the results of node 1 and node 2 do not
// mix. If no attempt passes, settled stops the test with the last problems.
func settled(t *testing.T, metric, nodeName string, checkFn func([]otelmetrics.MetricResult) []string, ready func() []string) []otelmetrics.MetricResult {
	t.Helper()
	selector := fmt.Sprintf(`{"__name__"=%q,"@resource.k8s.cluster.name"=%q,"@resource.k8s.node.name"=%q}`,
		metric, cfg.ClusterName, nodeName)
	var problems []string
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(retryInterval)
		}
		if ready != nil {
			if problems = ready(); len(problems) > 0 {
				continue
			}
		}
		results, err := client.Query(context.Background(), selector)
		if err != nil {
			problems = []string{err.Error()}
			continue
		}
		problems = checkFn(results)
		if len(problems) == 0 {
			return results
		}
	}
	t.Fatalf("%s from node %s after %d attempts:\n%s", metric, nodeName, attempts, strings.Join(problems, "\n"))
	return nil
}

// freeNodeReady returns a problem until the agent on node 2 has run for
// staleWindow, so the pod-less series of its first scrapes left the lookback.
func freeNodeReady() []string {
	start, err := agentStart(context.Background(), freeNode)
	if err != nil {
		return []string{err.Error()}
	}
	if age := time.Since(start); age < staleWindow {
		return []string{fmt.Sprintf("agent on node %s started %s ago, waiting for %s", freeNode, age.Round(time.Second), staleWindow)}
	}
	return nil
}

// checkFree returns the failures of checks 1 and 2 on node 2.
func checkFree(results []otelmetrics.MetricResult) []string {
	byDevice := map[string][]otelmetrics.MetricResult{}
	for _, r := range results {
		byDevice[device(r)] = append(byDevice[device(r)], r)
	}
	names := make([]string, 0, len(byDevice))
	for d := range byDevice {
		names = append(names, d)
	}
	sort.Strings(names)
	if len(names) != expectedEFACount {
		return []string{fmt.Sprintf("check 1: want %d aws.efa.device values, got %d: %q", expectedEFACount, len(names), names)}
	}
	for _, d := range names {
		if !all(byDevice[d], hasNoPodKeys) {
			return []string{fmt.Sprintf("check 2: want no device with %q at resource or data point scope: %s",
				podKeys, describe(byDevice, names))}
		}
	}
	return nil
}

// check returns the failures of checks 1 to 3.
func check(results []otelmetrics.MetricResult) []string {
	byDevice := map[string][]otelmetrics.MetricResult{}
	for _, r := range results {
		byDevice[device(r)] = append(byDevice[device(r)], r)
	}
	names := make([]string, 0, len(byDevice))
	for d := range byDevice {
		names = append(names, d)
	}
	sort.Strings(names)
	if len(names) != expectedEFACount {
		return []string{fmt.Sprintf("check 1: want %d aws.efa.device values, got %d: %q", expectedEFACount, len(names), names)}
	}

	// kept has the devices whose series all carry the efa-keep pod and
	// namespace. stripped has the devices without any pod key.
	var kept, stripped []string
	for _, d := range names {
		switch {
		case all(byDevice[d], hasKeepPod):
			kept = append(kept, d)
		case all(byDevice[d], hasNoPodKeys):
			stripped = append(stripped, d)
		}
	}
	if len(kept) == 0 {
		return []string{fmt.Sprintf("check 2: no device has the efa-keep pod %q in namespace %q on every series: %s",
			keepPod, keepNamespace, describe(byDevice, names))}
	}
	if len(stripped) != 1 {
		return []string{fmt.Sprintf("check 3: want 1 device without %q at resource or data point scope, got %q: %s",
			podKeys, stripped, describe(byDevice, names))}
	}
	return nil
}

// hasKeepPod reports whether the series carries the efa-keep pod and
// namespace at resource scope.
func hasKeepPod(r otelmetrics.MetricResult) bool {
	return r.Labels.Resource["k8s.pod.name"] == keepPod && r.Labels.Resource["k8s.namespace.name"] == keepNamespace
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

// describe lists the resource pod and namespace of each series, per device.
func describe(byDevice map[string][]otelmetrics.MetricResult, names []string) string {
	var parts []string
	for _, d := range names {
		for _, r := range byDevice[d] {
			parts = append(parts, fmt.Sprintf("device %q: pod %q, namespace %q", d,
				r.Labels.Resource["k8s.pod.name"], r.Labels.Resource["k8s.namespace.name"]))
		}
	}
	return strings.Join(parts, "; ")
}

// device returns the series' aws.efa.device, from resource or datapoint scope.
func device(r otelmetrics.MetricResult) string {
	if v := r.Labels.Resource["aws.efa.device"]; v != "" {
		return v
	}
	return r.Labels.Datapoint["aws.efa.device"]
}
