//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// queryWorkloadMetric queries a per-node workload metric scoped to this cluster,
// the workload namespace, and one workload (app=sm-app|pm-app), retrying until
// results appear or ctx is done (CloudWatch ingestion lags scrape time). Filtering
// by app yields one path's series so the ServiceMonitor and PodMonitor paths can be
// asserted independently.
func queryWorkloadMetric(ctx context.Context, t *testing.T, metricName, app string) []otelmetrics.MetricResult {
	t.Helper()
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.namespace.name"="%s","%s"="%s"}`,
		metricName, esc.Replace(cfg.ClusterName), esc.Replace(workloadNamespace), workloadAppLabel, esc.Replace(app))

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		results, err := client.Query(ctx, promql)
		if err == nil && len(results) > 0 {
			return results
		}
		select {
		case <-ctx.Done():
			t.Logf("no results for %s{%s=%s} before context done (err=%v)", metricName, workloadAppLabel, app, err)
			return nil
		case <-ticker.C:
		}
	}
}

// workloadResults finds the first perNodeMetrics series available for the given
// workload app, returning the results and the metric name used.
func workloadResults(ctx context.Context, t *testing.T, app string) ([]otelmetrics.MetricResult, string) {
	t.Helper()
	for _, m := range perNodeMetrics {
		if r := queryWorkloadMetric(ctx, t, m, app); len(r) > 0 {
			return r, m
		}
	}
	return nil, ""
}

// workloadJobs are the Target Allocator job names for the two per-node workload
// monitors in resources/workload.yaml: sm-app through the ServiceMonitor and
// pm-app through the PodMonitor. Each is asserted on its own so a failing path
// can't hide behind the other.
var workloadJobs = map[string]string{
	"sm-app": "serviceMonitor/" + workloadNamespace + "/sm-app/0",
	"pm-app": "podMonitor/" + workloadNamespace + "/pm-app/0",
}

// podNodeLabel is the discovery label carrying the node of the scraped pod.
const podNodeLabel = "__meta_kubernetes_pod_node_name"

// taTargetGroup is one entry of a Target Allocator /jobs/<job>/targets response.
type taTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// taCollectorTargets is one collector's entry in a /jobs/<job>/targets response.
type taCollectorTargets struct {
	Targets []taTargetGroup `json:"targets"`
}

// assignment is one target as allocated by the Target Allocator: the scraped
// pod's node and the collector (agent pod) it was assigned to.
type assignment struct {
	target, targetNode, collector string
	// selected is true when the target matches the monitor (the workload's app
	// label on its metrics port). The Target Allocator allocates discovery targets
	// before the monitor's relabel rules run, so a job also carries the other pods
	// in the namespace, which the agent drops at scrape time.
	selected bool
}

// perNodeAssignments reads, from the per-node Target Allocator, which collector
// each target of every workload job was assigned to. It asks the allocator
// directly instead of inferring it from exported metrics: the Prometheus receiver
// sets the resource k8s.node.name from the scraped target's discovery labels,
// so a metric cannot show which agent scraped it.
func perNodeAssignments(t *testing.T, clientset *kubernetes.Clientset) map[string][]assignment {
	t.Helper()
	var script strings.Builder
	script.WriteString("for i in $(seq 1 12); do\n  ok=1\n")
	for app, job := range workloadJobs {
		u := fmt.Sprintf("https://%s.%s.svc:80/jobs/%s/targets", perNodeTAService, agentNamespace, url.PathEscape(job))
		fmt.Fprintf(&script, "  r_%s=$(%s %q || true)\n", strings.ReplaceAll(app, "-", "_"), taCurl, u)
		fmt.Fprintf(&script, "  echo \"$r_%s\" | grep -q %s || ok=0\n", strings.ReplaceAll(app, "-", "_"), podNodeLabel)
	}
	script.WriteString("  [ $ok = 1 ] && break\n  sleep 15\ndone\n")
	for app := range workloadJobs {
		v := strings.ReplaceAll(app, "-", "_")
		fmt.Fprintf(&script, "echo \"===BEGIN %s===\"; echo \"$r_%s\"; echo \"===END %s===\"\n", app, v, app)
	}
	logs := runTAProbe(t, clientset, script.String())

	out := map[string][]assignment{}
	for app := range workloadJobs {
		raw := strings.TrimSpace(between(logs, "===BEGIN "+app+"===", "===END "+app+"==="))
		require.NotEmptyf(t, raw, "per-node Target Allocator returned nothing for job %q", workloadJobs[app])
		var byCollector map[string]taCollectorTargets
		require.NoErrorf(t, json.Unmarshal([]byte(raw), &byCollector),
			"parsing /jobs/%s/targets: %s", workloadJobs[app], raw)
		for collector, ct := range byCollector {
			for _, g := range ct.Targets {
				for _, tgt := range g.Targets {
					out[app] = append(out[app], assignment{
						target:     tgt,
						targetNode: g.Labels[podNodeLabel],
						collector:  collector,
						selected: g.Labels["__meta_kubernetes_pod_label_app"] == app &&
							g.Labels["__meta_kubernetes_pod_container_port_name"] == "metrics",
					})
				}
			}
		}
	}
	return out
}

// collectorNodes maps each per-node agent pod (Target Allocator collector ID) to
// the node it runs on, from the Kubernetes API.
func collectorNodes(t *testing.T, clientset *kubernetes.Clientset) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pods, err := clientset.CoreV1().Pods(agentNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=" + agentNamespace + ".cloudwatch-agent,app.kubernetes.io/component=amazon-cloudwatch-agent",
	})
	require.NoError(t, err, "listing per-node agent pods")
	out := make(map[string]string, len(pods.Items))
	for _, p := range pods.Items {
		out[p.Name] = p.Spec.NodeName
	}
	require.NotEmpty(t, out, "no per-node agent pods found")
	return out
}

// readyWorkloadPods counts the Running pods of a workload app.
func readyWorkloadPods(t *testing.T, clientset *kubernetes.Clientset, app string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pods, err := clientset.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + app})
	require.NoErrorf(t, err, "listing %s pods", app)
	n := 0
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			n++
		}
	}
	return n
}

// TestPerNodeAllocation verifies the per-node strategy at the Target Allocator for
// BOTH the ServiceMonitor (sm-app) and PodMonitor (pm-app) paths: every target the
// allocator hands out is assigned to the per-node agent on the same node as its
// pod, and every running workload pod is assigned to exactly one agent.
func TestPerNodeAllocation(t *testing.T) {
	clientset := k8sClientset(t)
	gt := getGroundTruth(t)
	nodeOf := collectorNodes(t, clientset)
	byApp := perNodeAssignments(t, clientset)

	for app := range workloadJobs {
		t.Run(app, func(t *testing.T) {
			got := byApp[app]
			require.NotEmptyf(t, got, "app=%q: the Target Allocator assigned no targets", app)

			seen := map[string]int{}
			selected := 0
			for _, a := range got {
				seen[a.target]++
				if a.selected {
					selected++
				}
				require.NotEmptyf(t, a.targetNode, "target %s has no %s label", a.target, podNodeLabel)
				_, known := gt.nodes[a.targetNode]
				require.Truef(t, known, "target %s is on unknown node %q", a.target, a.targetNode)
				agentNode, ok := nodeOf[a.collector]
				require.Truef(t, ok, "target %s assigned to %q, which is not a per-node agent pod", a.target, a.collector)
				require.Equalf(t, a.targetNode, agentNode,
					"target %s (pod on %s) assigned to %s on %s: not node-local", a.target, a.targetNode, a.collector, agentNode)
			}
			for tgt, n := range seen {
				require.Equalf(t, 1, n, "target %s assigned to %d agents", tgt, n)
			}
			want := readyWorkloadPods(t, clientset, app)
			require.Equalf(t, want, selected, "app=%q: %d running pods but %d matching targets assigned", app, want, selected)
			t.Logf("per-node OK for app=%q: %d targets (%d matching the monitor), each assigned to the agent on its own node",
				app, len(got), selected)
		})
	}
}

// TestPerNodeCoverageAcrossNodes asserts each workload's targets span more than
// one node, so the per-node check above is meaningful (a single-node placement
// would pass trivially).
func TestPerNodeCoverageAcrossNodes(t *testing.T) {
	gt := getGroundTruth(t)
	if len(gt.nodes) < 2 {
		t.Skipf("cluster has %d node(s); per-node spread is only meaningful with >= 2", len(gt.nodes))
	}
	byApp := perNodeAssignments(t, k8sClientset(t))
	for app := range workloadJobs {
		t.Run(app, func(t *testing.T) {
			nodes := map[string]struct{}{}
			for _, a := range byApp[app] {
				if a.selected {
					nodes[a.targetNode] = struct{}{}
				}
			}
			require.GreaterOrEqualf(t, len(nodes), 2,
				"app=%q targets only on %d node(s) (%v); expected spread across >= 2", app, len(nodes), keys(nodes))
		})
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
