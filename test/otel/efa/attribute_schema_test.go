//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package efa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestAttributeSchema — the closed set of resource and instrumentation-scope
// attributes on EFA series. Every series must have every required key and
// nothing undeclared, expected keys must be on at least one series, and keys set
// together must arrive together. Node and pod labels are open by construction
// (k8sattributes extracts "(.*)"). Datapoint scope is not covered.
// ---------------------------------------------------------------------------

// awsdevicepodcorrelation then efa_promote; only on devices a pod holds.
var efaPodKeys = []string{"k8s.pod.name", "k8s.namespace.name", "k8s.container.name"}

// awsefareceiver sets no resource attributes, so unlike the scraped pipelines
// there are no server.* or service.* keys. This suite runs no workload that holds
// a device, so the pod keys are optional rather than expected.
var efaResourceSchema = otelmetrics.KeySchema{
	Required: otelmetrics.ConcatKeys(otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
		"k8s.cluster.name", // set_cluster_name; protected
		"k8s.node.name",    // set_node_name, promote_node_name; protected
		"k8s.node.uid",     // k8sattributes/cw_k8s_ci_v0_node
	}),
	Optional:     otelmetrics.ConcatKeys(efaPodKeys, []string{"k8s.pod.uid"}, otelmetrics.OwnerKindKeys, otelmetrics.WorkloadKeys),
	OpenPrefixes: []string{"k8s.node.label.", "k8s.pod.label."},
	Implies:      otelmetrics.MergeImplies(otelmetrics.WorkloadImplies(), otelmetrics.Together(efaPodKeys...)),
}

func TestAttributeSchema(t *testing.T) {
	t.Parallel()
	tally, err := otelmetrics.TallySchema(context.Background(), queryCache, efaMetricNamesList, otelmetrics.GroupSchema{
		Resource:        efaResourceSchema,
		Instrumentation: otelmetrics.KeySchema{Required: otelmetrics.ScopeKeys}, // set_scope_efa
	}, nil)
	require.NoError(t, err, "no EFA nodes?")
	t.Logf("%d series across %d metrics", tally.Series, len(efaMetricNamesList))
	require.NotZero(t, tally.Series, "no series checked")
	for _, p := range tally.Problems() {
		t.Errorf("%s: change the schema if intended", p)
	}
}
