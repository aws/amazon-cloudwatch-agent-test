//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
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
// every node and pod label the running denylist does not match must arrive with
// the same value, and every label it matches must not. The expectation comes
// from the cluster and the live configuration; the one maintained list is the
// never-denied attributes, which stops an over-broad denylist entry passing.
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

// agentDenylist reads the attribute-limit configuration an agent is running.
func agentDenylist(t *testing.T, configMap string) otelmetrics.AttributeDenylist {
	t.Helper()
	cm, err := k8sClientset(t).CoreV1().ConfigMaps(agentNamespace).Get(
		context.Background(), configMap, metav1.GetOptions{})
	require.NoError(t, err, "reading ConfigMap %s/%s", agentNamespace, configMap)
	raw, ok := cm.Data[otelConfigKey]
	require.True(t, ok, "ConfigMap %s has no %s", configMap, otelConfigKey)
	d, err := otelmetrics.ParseAttributeDenylist(raw)
	require.NoError(t, err, "ConfigMap %s", configMap)
	return d
}

func TestDenylistSparesNeverDeniedAttributes(t *testing.T) {
	for _, configMap := range []string{nodeAgentConfigMap, clusterScraperConfigMap} {
		t.Run(configMap, func(t *testing.T) {
			d := agentDenylist(t, configMap)
			for _, v := range d.NeverDeniedViolations() {
				t.Errorf("%s: %s", d.Processor, v)
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
func reconcileTwice(t *testing.T, kind string, pass reconcilePass) {
	t.Helper()
	first := pass(t)
	t.Logf("reconciled %d %ss, %d labels, %d series skipped at the attribute cap, %d failed, %d of %d required with no series",
		first.Objects, kind, first.Labels, first.AtCap, len(first.Failed), len(first.NoSeries), first.Required)
	require.NotZero(t, first.Required, "the Kubernetes API listed no %s that must have a series", kind)
	require.NotZero(t, first.Objects, "no series matched a %s from the Kubernetes API", kind)
	require.NotZero(t, first.Labels, "%ss matched but carried no checkable label", kind)
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
		t.Errorf("%s %s: missing %v, value mismatch %v, denied but present %v",
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

// freshGroundTruth lists the nodes and pods now, unlike the session-wide getGroundTruth.
func freshGroundTruth(t *testing.T) *k8sGroundTruth {
	t.Helper()
	gt, err := snapshotGroundTruth(k8sClientset(t))
	require.NoError(t, err, "listing nodes and pods")
	return gt
}

// queryAllSeries returns every series of a metric in this cluster, on every node,
// evaluated at one time.
func queryAllSeries(t *testing.T, metric string) []otelmetrics.MetricResult {
	t.Helper()
	results, err := client.QueryComplete(context.Background(), fmt.Sprintf(
		`{"__name__"="%s","@resource.k8s.cluster.name"="%s"}`,
		otelmetrics.EscapePromQLValue(metric), otelmetrics.EscapePromQLValue(cfg.ClusterName)))
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
			d := agentDenylist(t, p.configMap)
			reconcileTwice(t, "node", func(t *testing.T) otelmetrics.ObjectReconciliation {
				gt := freshGroundTruth(t)
				return otelmetrics.ReconcileObjects(queryAllSeries(t, p.metric),
					func(l otelmetrics.MetricLabels) (string, map[string]string, bool) {
						name := l.Resource["k8s.node.name"]
						node, ok := gt.nodes[name] // absent if it joined or left between the two reads
						return name, node.Labels, ok
					}, p.required(gt), "k8s.node.label.", d)
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
			d := agentDenylist(t, p.configMap)
			reconcileTwice(t, "pod", func(t *testing.T) otelmetrics.ObjectReconciliation {
				gt := freshGroundTruth(t)
				return otelmetrics.ReconcileObjects(queryAllSeries(t, p.metric),
					func(l otelmetrics.MetricLabels) (string, map[string]string, bool) {
						key := l.Resource["k8s.namespace.name"] + "/" + l.Resource["k8s.pod.name"]
						pod, ok := gt.pods[key] // absent if it was created or deleted between the two reads
						return key, pod.Labels, ok
					}, p.required(gt), "k8s.pod.label.", d)
			})
		})
	}
}
