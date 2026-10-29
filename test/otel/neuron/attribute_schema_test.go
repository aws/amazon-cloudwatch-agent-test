//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestAttributeSchema — the closed set of resource and instrumentation-scope
// attributes on Neuron series. Every series must have every required key and
// nothing undeclared, expected keys must be on at least one series, and keys set
// together must arrive together. Node and pod labels are open by construction
// (k8sattributes extracts "(.*)"). Datapoint scope is not covered.
// ---------------------------------------------------------------------------

// awsdevicepodcorrelation then groupbyattrs; only on cores or devices a pod holds.
var neuronPodKeys = []string{"k8s.pod.name", "k8s.namespace.name", "k8s.container.name"}

var neuronResourceSchema = otelmetrics.KeySchema{
	Required: otelmetrics.ConcatKeys(otelmetrics.PrometheusReceiverKeys, otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
		"k8s.cluster.name", // set_cluster_name; protected
		"k8s.node.name",    // set_node_name, promote_node_name; protected
		"k8s.node.uid",     // k8sattributes/cw_k8s_ci_v0_node
	}),
	// The neuron-burn Deployments hold cores and devices and run a Neuron runtime.
	Expected: otelmetrics.ConcatKeys(neuronPodKeys, []string{
		"aws.neuron.runtime.tag", // neuron_promote; zero-filled series of an idle device have no runtime_tag
		"k8s.deployment.name", "k8s.replicaset.name",
	}, otelmetrics.WorkloadKeys),
	Optional: otelmetrics.ConcatKeys([]string{"k8s.pod.uid"}, // k8sattributes/cw_k8s_ci_v0_pod
		otelmetrics.Without(otelmetrics.OwnerKindKeys, "k8s.deployment.name", "k8s.replicaset.name")),
	OpenPrefixes: []string{"k8s.node.label.", "k8s.pod.label."},
	Implies:      otelmetrics.MergeImplies(otelmetrics.WorkloadImplies(), otelmetrics.Together(neuronPodKeys...)),
}

func TestAttributeSchema(t *testing.T) {
	t.Parallel()
	tally, err := otelmetrics.TallySchema(context.Background(), queryCache, neuronMetricNamesList, otelmetrics.GroupSchema{
		Resource:        neuronResourceSchema,
		Instrumentation: otelmetrics.KeySchema{Required: otelmetrics.ScopeKeys}, // set_scope_neuron_monitor
	}, nil)
	require.NoError(t, err, "no Neuron nodes?")
	t.Logf("%d series across %d metrics", tally.Series, len(neuronMetricNamesList))
	require.NotZero(t, tally.Series, "no series checked")
	for _, p := range tally.Problems() {
		t.Errorf("%s: change the schema if intended", p)
	}
}
