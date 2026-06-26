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

// queryWorkloadMetric queries a per-node workload metric scoped to this cluster
// and the workload namespace, retrying until results appear or the deadline
// passes (CloudWatch ingestion lags scrape time).
func queryWorkloadMetric(t *testing.T, metricName string) []otelmetrics.MetricResult {
	t.Helper()
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	promql := fmt.Sprintf(`%s{"@resource.k8s.cluster.name"="%s","@resource.k8s.namespace.name"="%s"}`,
		metricName, esc.Replace(cfg.ClusterName), esc.Replace(workloadNamespace))

	deadline := time.Now().Add(5 * time.Minute)
	for {
		results, err := client.Query(context.Background(), promql)
		if err == nil && len(results) > 0 {
			return results
		}
		if time.Now().After(deadline) {
			t.Logf("no results for %s after retry window (err=%v)", metricName, err)
			return nil
		}
		time.Sleep(15 * time.Second)
	}
}

// TestPerNodeAllocation verifies the per-node strategy end to end: every scraped
// series must be collected by the agent running on the SAME node as the scraped
// pod, i.e. the relabeled target_node equals the scraping agent's
// @resource.k8s.node.name.
func TestPerNodeAllocation(t *testing.T) {
	gt := getGroundTruth(t)

	var results []otelmetrics.MetricResult
	var usedMetric string
	for _, m := range perNodeMetrics {
		if r := queryWorkloadMetric(t, m); len(r) > 0 {
			results, usedMetric = r, m
			break
		}
	}
	require.NotEmpty(t, results, "no per-node workload metrics found in CloudWatch for namespace %q; "+
		"expected one of %v carrying the target_node relabel", workloadNamespace, perNodeMetrics)
	t.Logf("validating per-node locality on %d series of %q", len(results), usedMetric)

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

	require.Zero(t, mismatches, "%d/%d series were scraped cross-node (per-node strategy not honored)",
		mismatches, len(results))
	require.Zero(t, missingLabels, "%d/%d series were missing target_node/node labels",
		missingLabels, len(results))

	t.Logf("per-node OK: %d series, all node-local, across %d node(s): %v",
		len(results), len(targetNodesSeen), keys(targetNodesSeen))
}

// TestPerNodeCoverageAcrossNodes asserts the workload is actually spread across
// more than one node, so the per-node check above is meaningful (a single-node
// cluster would pass trivially).
func TestPerNodeCoverageAcrossNodes(t *testing.T) {
	gt := getGroundTruth(t)
	if len(gt.nodes) < 2 {
		t.Skipf("cluster has %d node(s); per-node spread is only meaningful with >= 2", len(gt.nodes))
	}

	var results []otelmetrics.MetricResult
	for _, m := range perNodeMetrics {
		if r := queryWorkloadMetric(t, m); len(r) > 0 {
			results = r
			break
		}
	}
	require.NotEmpty(t, results, "no per-node workload metrics found to assess node spread")

	seen := map[string]struct{}{}
	for _, r := range results {
		if tn := r.Labels.Datapoint[targetNodeLabel]; tn != "" {
			seen[tn] = struct{}{}
		}
	}
	require.GreaterOrEqual(t, len(seen), 2,
		"per-node workload only observed on %d node(s) (%v); expected spread across >= 2", len(seen), keys(seen))
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
