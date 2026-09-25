//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

// TestPerNodeAllocation verifies the per-node strategy end to end for BOTH the
// ServiceMonitor (sm-app) and PodMonitor (pm-app) paths: every scraped series must
// be collected by the agent running on the SAME node as the scraped pod, i.e. the
// relabeled target_node equals the scraping agent's @resource.k8s.node.name. Each
// path is asserted independently so a broken path can't hide behind the other.
func TestPerNodeAllocation(t *testing.T) {
	gt := getGroundTruth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	for _, app := range workloadApps {
		t.Run(app, func(t *testing.T) {
			results, usedMetric := workloadResults(ctx, t, app)
			require.NotEmptyf(t, results,
				"no per-node metrics in CloudWatch for app=%q (expected one of %v carrying target_node); "+
					"the %s scrape path may not be discovering/scraping", app, perNodeMetrics, app)
			t.Logf("validating per-node locality on %d series of %q for app=%q", len(results), usedMetric, app)

			targetNodesSeen := map[string]struct{}{}
			var mismatches, missingLabels int
			for _, r := range results {
				targetNode := r.Labels.Datapoint[targetNodeLabel]
				agentNode := r.Labels.Resource[agentNodeResourceKey]

				if targetNode == "" || agentNode == "" {
					missingLabels++
					t.Errorf("series missing node labels: target_node=%q agent(%s)=%q labels=%v",
						targetNode, agentNodeResourceKey, agentNode, r.Labels.AllLabels())
					continue
				}
				targetNodesSeen[targetNode] = struct{}{}

				// The scraped pod's node must be a real cluster node.
				if _, ok := gt.nodes[targetNode]; !ok {
					t.Errorf("target_node %q is not a known cluster node", targetNode)
				}

				// Core per-node invariant: scraped by the agent on the pod's own node.
				if targetNode != agentNode {
					mismatches++
					t.Errorf("per-node violation: target_node=%q scraped by agent on node=%q (cross-node)",
						targetNode, agentNode)
				}
			}

			require.Zerof(t, mismatches, "%d/%d series were scraped cross-node for app=%q (per-node strategy not honored)",
				mismatches, len(results), app)
			require.Zerof(t, missingLabels, "%d/%d series were missing target_node/node labels for app=%q",
				missingLabels, len(results), app)

			t.Logf("per-node OK for app=%q: %d series, all node-local, across %d node(s): %v",
				app, len(results), len(targetNodesSeen), keys(targetNodesSeen))
		})
	}
}

// TestPerNodeCoverageAcrossNodes asserts each workload is actually spread across
// more than one node, so the per-node check above is meaningful (a single-node
// cluster would pass trivially).
func TestPerNodeCoverageAcrossNodes(t *testing.T) {
	gt := getGroundTruth(t)
	if len(gt.nodes) < 2 {
		t.Skipf("cluster has %d node(s); per-node spread is only meaningful with >= 2", len(gt.nodes))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	for _, app := range workloadApps {
		t.Run(app, func(t *testing.T) {
			results, _ := workloadResults(ctx, t, app)
			require.NotEmptyf(t, results, "no per-node metrics found to assess node spread for app=%q", app)

			seen := map[string]struct{}{}
			for _, r := range results {
				if tn := r.Labels.Datapoint[targetNodeLabel]; tn != "" {
					seen[tn] = struct{}{}
				}
			}
			require.GreaterOrEqualf(t, len(seen), 2,
				"app=%q only observed on %d node(s) (%v); expected spread across >= 2", app, len(seen), keys(seen))
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
