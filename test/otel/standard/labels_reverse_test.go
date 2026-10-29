//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// Reverse label reconciliation. TestNodeLabelsMatchKubernetesAPI checks that
// every k8s.node.label.* on a metric exists on the node; it cannot notice a
// label that never arrived. These tests start from the Kubernetes API instead:
// every node and pod label that the running k8sattributes rules extract and the
// running denylist does not match must arrive with the same value, and every
// other label must not. The expectation comes from the cluster and the live
// configuration; the one maintained list is the never-denied attributes, which
// stops an over-broad denylist entry passing.
//
// Non-default filter values change the expectation. A label include list
// narrows the extraction rules, so a label it leaves out must not arrive. A
// namespace filter drops the series of every pod in a namespace it removes, so
// those pods need no series. Node series carry no k8s.namespace.name, so the
// namespace filter keeps them. A filter the model cannot read skips the test
// with the reason.
//
// Labels reach metrics on two paths, each with its own k8sattributes extraction
// and denylist: the cluster scraper's kube-state-metrics pipeline, which reports
// every node and pod, and the node agent's kubeletstats pipeline, which reports
// the nodes and pods where the agent runs. Both are queried cluster-wide, so the
// clusterHostTypes pin does not limit them. Every object a path reports must
// have a series, or an object that lost all of its series would pass: every
// node and pod on the cluster scraper path, and on the node agent path every
// node with a ready agent pod and every running pod on such a node. Each pass
// reads the API, then every series at one evaluation time, so the two sides are
// seconds apart. An object fails only if it fails two passes recheckDelay apart,
// because a label change or a new object reaches the API before the next
// collection is ingested.
// ---------------------------------------------------------------------------

const (
	agentNamespace          = "amazon-cloudwatch"
	nodeAgentConfigMap      = "cloudwatch-agent"
	nodeAgentDaemonSet      = "cloudwatch-agent"
	clusterScraperConfigMap = "cloudwatch-agent-cluster-scraper"
	otelConfigKey           = "cwagentotelconfig.yaml"

	// recheckDelay covers one collection interval of either agent plus ingestion.
	recheckDelay = 90 * time.Second
)

// agentConfig reads the collector configuration an agent is running.
func agentConfig(t *testing.T, configMap string) string {
	t.Helper()
	cm, err := k8sClientset(t).CoreV1().ConfigMaps(agentNamespace).Get(
		context.Background(), configMap, metav1.GetOptions{})
	require.NoError(t, err, "reading ConfigMap %s/%s", agentNamespace, configMap)
	raw, ok := cm.Data[otelConfigKey]
	require.True(t, ok, "ConfigMap %s has no %s", configMap, otelConfigKey)
	return raw
}

// agentDenylist reads the attribute-limit configuration an agent is running.
func agentDenylist(t *testing.T, configMap string) otelmetrics.AttributeDenylist {
	t.Helper()
	d, err := otelmetrics.ParseAttributeDenylist(agentConfig(t, configMap))
	require.NoError(t, err, "ConfigMap %s", configMap)
	return d
}

// labelFilters is what an agent's configuration does on one label path.
type labelFilters struct {
	extraction otelmetrics.LabelExtraction
	denylist   otelmetrics.AttributeDenylist
	namespaces otelmetrics.NamespaceFilter // read for pods only
}

// agentLabelFilters reads the filters an agent runs for labels from "node" or
// "pod". It skips the test for a filter the model cannot read.
func agentLabelFilters(t *testing.T, configMap, from string) labelFilters {
	t.Helper()
	raw := agentConfig(t, configMap)
	var f labelFilters
	var err error
	f.denylist, err = otelmetrics.ParseAttributeDenylist(raw)
	require.NoError(t, err, "ConfigMap %s", configMap)
	f.extraction, err = otelmetrics.ParseLabelExtraction(raw, from)
	skipIfNotModeled(t, configMap, err)
	t.Logf("ConfigMap %s: %s labels extracted by %s, every label: %v", configMap, from, f.extraction.Processor, f.extraction.ExtractsAll())
	if from == "pod" {
		f.namespaces, err = otelmetrics.ParseNamespaceFilter(raw)
		skipIfNotModeled(t, configMap, err)
		t.Logf("ConfigMap %s: namespace filter processors %v", configMap, f.namespaces.Processors)
	}
	return f
}

func skipIfNotModeled(t *testing.T, configMap string, err error) {
	t.Helper()
	if errors.Is(err, otelmetrics.ErrFilterNotModeled) {
		t.Skipf("ConfigMap %s: %v. The test cannot tell which labels or objects this filter removes", configMap, err)
	}
	require.NoError(t, err, "ConfigMap %s", configMap)
}

// TestDenylistSparesNeverDeniedAttributes fails for a denylist entry that removes
// a never-denied attribute. A customer may remove a protected label with
// nodeLabels.exclude or podLabels.exclude, so the entries that the declared
// clusterLabelExcludes render to are logged instead.
func TestDenylistSparesNeverDeniedAttributes(t *testing.T) {
	for _, configMap := range []string{nodeAgentConfigMap, clusterScraperConfigMap} {
		t.Run(configMap, func(t *testing.T) {
			d := agentDenylist(t, configMap)
			chart, customer := d.SplitCustomerExcludes(clusterLabelExcludes)
			if entries := customer.Entries(); len(entries) > 0 {
				t.Logf("%s: %d entries match a declared label exclude: %v", d.Processor, len(entries), entries)
			}
			for _, v := range customer.NeverDeniedViolations() {
				t.Logf("%s: declared label exclude, allowed: %s", d.Processor, v)
			}
			for _, v := range chart.NeverDeniedViolations() {
				t.Errorf("%s: %s (if a label exclude of the deployment adds it, declare the exclude in %s or %s)",
					d.Processor, v, otelmetrics.PodLabelExcludesEnvVar, otelmetrics.NodeLabelExcludesEnvVar)
			}
		})
	}
}

// reconcilePass reads both sides once and reconciles them.
type reconcilePass func(t *testing.T) otelmetrics.ObjectReconciliation

// failedObjects returns the objects that failed a pass: a series that did not
// reconcile, or a required object with no series.
func failedObjects(r otelmetrics.ObjectReconciliation) map[string]bool {
	failed := make(map[string]bool, len(r.Failed)+len(r.NoSeries))
	for key := range r.Failed {
		failed[key] = true
	}
	for _, key := range r.NoSeries {
		failed[key] = true
	}
	return failed
}

// reconcileTwice runs pass and, if any object failed, runs it again after
// recheckDelay. Only objects that fail both passes are reported.
func reconcileTwice(t *testing.T, kind string, e otelmetrics.LabelExtraction, pass reconcilePass) {
	t.Helper()
	first := pass(t)
	t.Logf("reconciled %d %ss, %d labels, %d series skipped at the attribute cap, %d failed, %d of %d required with no series",
		first.Objects, kind, first.Labels, first.AtCap, len(first.Failed), len(first.NoSeries), first.Required)
	require.NotZero(t, first.Required, "the Kubernetes API listed no %s that must have a series", kind)
	require.NotZero(t, first.Objects, "no series matched a %s from the Kubernetes API", kind)
	if first.Labels == 0 && !e.ExtractsAll() {
		t.Logf("the label include list in %s extracts no label that these %ss carry", e.Processor, kind)
	} else {
		require.NotZero(t, first.Labels, "%ss matched but carried no checkable label", kind)
	}
	failedBefore := failedObjects(first)
	if len(failedBefore) == 0 {
		return
	}
	t.Logf("re-reading the API and the metrics in %s to separate label churn and new objects from a real drop", recheckDelay)
	time.Sleep(recheckDelay)
	second := pass(t)
	keys := make([]string, 0, len(failedBefore))
	for key := range failedObjects(second) {
		if failedBefore[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		diff, hasSeries := second.Failed[key]
		if !hasSeries {
			t.Errorf("%s %s: no series", kind, key)
			continue
		}
		t.Errorf("%s %s: missing %v, value mismatch %v, removed but present %v",
			kind, key, diff.Missing, diff.Mismatched, diff.Leaked)
	}
}

// agentNodes returns the listed nodes where a node agent pod is ready.
func agentNodes(gt *k8sGroundTruth) map[string]bool {
	nodes := map[string]bool{}
	for _, p := range gt.pods {
		if p.Namespace != agentNamespace || p.DeletionTimestamp != nil || !ownedByDaemonSet(p, nodeAgentDaemonSet) {
			continue
		}
		if _, listed := gt.nodes[p.Spec.NodeName]; !listed {
			continue // it left between the two reads
		}
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				nodes[p.Spec.NodeName] = true
			}
		}
	}
	return nodes
}

func ownedByDaemonSet(p corev1.Pod, name string) bool {
	for _, o := range p.OwnerReferences {
		if o.Kind == "DaemonSet" && o.Name == name {
			return true
		}
	}
	return false
}

// everyNode and everyPod are the objects that must have a series on the cluster
// scraper path, because kube-state-metrics reports every object the API lists.
func everyNode(gt *k8sGroundTruth) []string {
	keys := make([]string, 0, len(gt.nodes))
	for name := range gt.nodes {
		keys = append(keys, name)
	}
	return keys
}

func everyPod(gt *k8sGroundTruth) []string {
	keys := make([]string, 0, len(gt.pods))
	for key := range gt.pods {
		keys = append(keys, key)
	}
	return keys
}

// nodesWithAgent and runningPodsWithAgent are the objects that must have a series
// on the node agent path, which reports its own node and the pods running there.
func nodesWithAgent(gt *k8sGroundTruth) []string {
	var keys []string
	for name := range agentNodes(gt) {
		keys = append(keys, name)
	}
	return keys
}

func runningPodsWithAgent(gt *k8sGroundTruth) []string {
	agents := agentNodes(gt)
	var keys []string
	for key, p := range gt.pods {
		// A terminating pod can stop reporting before the API drops it.
		if agents[p.Spec.NodeName] && p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil {
			keys = append(keys, key)
		}
	}
	return keys
}

// withoutFilteredNamespaces returns the pod keys whose namespace the filter keeps.
func withoutFilteredNamespaces(gt *k8sGroundTruth, keys []string, f otelmetrics.NamespaceFilter) []string {
	var kept []string
	for _, key := range keys {
		if !f.Drops(gt.pods[key].Namespace) {
			kept = append(kept, key)
		}
	}
	return kept
}

// freshGroundTruth lists the nodes and pods now, unlike the session-wide getGroundTruth.
func freshGroundTruth(t *testing.T) *k8sGroundTruth {
	t.Helper()
	gt, err := snapshotGroundTruth(k8sClientset(t))
	require.NoError(t, err, "listing nodes and pods")
	return gt
}

// queryAllSeries returns every series of a metric in this cluster, on every node,
// evaluated at one time. Unlike allNodeSeries it reads now, not from the cache, so a
// pass compares the API with metrics seconds apart.
func queryAllSeries(t *testing.T, metric string) []otelmetrics.MetricResult {
	t.Helper()
	results, err := allNodesCache.GetWithFilter(context.Background(), metric, nil)
	require.NoError(t, err, "querying %s", metric)
	require.NotEmpty(t, results, "%s not available", metric)
	return results
}

// labelPath is one metric that carries an object's labels, one series per object,
// the agent whose denylist the series went through, and the objects that must
// have a series.
type labelPath struct {
	agent, metric, configMap string
	required                 func(*k8sGroundTruth) []string
}

func TestNodeLabelsReachMetrics(t *testing.T) {
	for _, p := range []labelPath{
		{"cluster-scraper", "kube_node_info", clusterScraperConfigMap, everyNode},
		{"node-agent", "k8s.node.cpu.usage", nodeAgentConfigMap, nodesWithAgent},
	} {
		p := p
		t.Run(p.agent, func(t *testing.T) {
			f := agentLabelFilters(t, p.configMap, "node")
			reconcileTwice(t, "node", f.extraction, func(t *testing.T) otelmetrics.ObjectReconciliation {
				gt := freshGroundTruth(t)
				return otelmetrics.ReconcileObjects(queryAllSeries(t, p.metric),
					func(l otelmetrics.MetricLabels) (string, map[string]string, bool) {
						name := l.Resource["k8s.node.name"]
						node, ok := gt.nodes[name] // absent if it joined or left between the two reads
						return name, node.Labels, ok
					}, p.required(gt), f.extraction, f.denylist)
			})
		})
	}
}

func TestPodLabelsReachMetrics(t *testing.T) {
	for _, p := range []labelPath{
		{"cluster-scraper", "kube_pod_info", clusterScraperConfigMap, everyPod},
		{"node-agent", "k8s.pod.cpu.usage", nodeAgentConfigMap, runningPodsWithAgent},
	} {
		p := p
		t.Run(p.agent, func(t *testing.T) {
			f := agentLabelFilters(t, p.configMap, "pod")
			reconcileTwice(t, "pod", f.extraction, func(t *testing.T) otelmetrics.ObjectReconciliation {
				gt := freshGroundTruth(t)
				reported := p.required(gt)
				required := withoutFilteredNamespaces(gt, reported, f.namespaces)
				if len(required) == 0 && len(reported) > 0 {
					t.Skipf("the namespace filter %v removes all %d pods this path reports", f.namespaces.Processors, len(reported))
				}
				return otelmetrics.ReconcileObjects(queryAllSeries(t, p.metric),
					func(l otelmetrics.MetricLabels) (string, map[string]string, bool) {
						key := l.Resource["k8s.namespace.name"] + "/" + l.Resource["k8s.pod.name"]
						pod, ok := gt.pods[key] // absent if it was created or deleted between the two reads
						return key, pod.Labels, ok
					}, required, f.extraction, f.denylist)
			})
		})
	}
}
