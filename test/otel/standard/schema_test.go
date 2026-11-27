//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// ---------------------------------------------------------------------------
// TestAttributeSchema — the closed set of resource and instrumentation-scope
// attributes each metric group carries. Every series must have every required
// key and nothing undeclared, so an attribute removed by mistake fails here
// even though no test names it, and so does one that appears unnoticed. Node and
// pod labels are open by construction (k8sattributes extracts "(.*)") and are
// covered by TestNodeLabelsReachMetrics and TestPodLabelsReachMetrics instead.
//
// Every key carries the chart component that sets it. Required keys are on every
// series, expected keys on at least one series of the row, and an implication
// (otelmetrics.KeySchema.Implies) ties keys that are set together, so removing a
// key family from all series fails even where each series may lack it.
// Datapoint scope is not covered: its keys come from the scraped exporter and
// move with its version. Every series in the cluster is read through
// allNodesCache, so the clusterHostTypes pin does not limit the check.
//
// An intentional attribute change edits the row in the same change. To add a
// key, list it as optional, merge the chart change, then make it required. Keys
// marked "protected" are on the attribute-limit processor's never-drop list.
// ---------------------------------------------------------------------------

type groupSchema struct {
	name    string
	metrics func() []string
	schema  otelmetrics.GroupSchema
}

var scopeInstrumentation = otelmetrics.KeySchema{Required: otelmetrics.ScopeKeys}

var attributeSchemas = []groupSchema{
	{
		name:    "apiserver",
		metrics: func() []string { return metricNames(controlPlaneMetrics) },
		schema: otelmetrics.GroupSchema{
			Resource: otelmetrics.KeySchema{Required: otelmetrics.ConcatKeys(otelmetrics.PrometheusReceiverKeys, otelmetrics.CloudKeys, []string{
				"k8s.cluster.name",   // set_cluster_name; protected
				"k8s.component.name", // set_component, promote_component; protected
				"k8s.namespace.name", // Prometheus receiver, from Kubernetes service discovery; protected
			})},
			Instrumentation: scopeInstrumentation,
		},
	},
	{
		name:    "node-exporter",
		metrics: func() []string { return metricNames(nodeExporterMetrics) },
		schema: otelmetrics.GroupSchema{
			Resource: otelmetrics.KeySchema{
				Required: otelmetrics.ConcatKeys(otelmetrics.PrometheusReceiverKeys, otelmetrics.CloudKeys, otelmetrics.HostKeys, []string{
					"k8s.cluster.name", // set_cluster_name; protected
					"k8s.node.name",    // set_node_name, promote_node_name; protected
					"k8s.node.uid",     // k8sattributes/cw_k8s_ci_v0_node
				}),
				OpenPrefixes: []string{"k8s.node.label."},
			},
			Instrumentation: scopeInstrumentation,
		},
	},
}

func TestAttributeSchema(t *testing.T) {
	for _, g := range attributeSchemas {
		g := g
		t.Run(g.name, func(t *testing.T) {
			deleted := deletedNodes(t)
			tally, err := otelmetrics.TallySchema(context.Background(), allNodesCache, g.metrics(), g.schema, deleted.forgive)
			require.NoError(t, err)
			t.Logf("%d series across %d metrics; %d series of deleted nodes forgiven a missing k8s.node.uid",
				tally.Series, len(g.metrics()), deleted.forgiven)
			require.NotZero(t, tally.Series, "no series checked")
			for _, p := range tally.Problems() {
				t.Errorf("%s: add or change the %q row if intended", p, g.name)
			}
		})
	}
}

// deletedNodeForgiveness forgives the one problem a deleted node causes:
// k8sattributes forgets the node, so k8s.node.uid is no longer set, while the
// node's last series stay inside the query lookback window. The API is read only
// when a series lacks k8s.node.uid.
type deletedNodeForgiveness struct {
	t        *testing.T
	live     map[string]bool
	forgiven int
}

func deletedNodes(t *testing.T) *deletedNodeForgiveness {
	return &deletedNodeForgiveness{t: t}
}

func (d *deletedNodeForgiveness) forgive(l otelmetrics.MetricLabels, c otelmetrics.SeriesCheck) otelmetrics.SeriesCheck {
	node := l.Resource["k8s.node.name"]
	if _, hasUID := l.Resource["k8s.node.uid"]; hasUID || node == "" {
		return c
	}
	if d.live == nil {
		d.live = map[string]bool{}
		for name := range freshGroundTruth(d.t).nodes {
			d.live[name] = true
		}
	}
	if d.live[node] {
		return c
	}
	d.forgiven++
	c.Missing = otelmetrics.Without(c.Missing, "k8s.node.uid")
	var broken []string
	for _, b := range c.Broken {
		if !strings.HasSuffix(b, " without k8s.node.uid") {
			broken = append(broken, b)
		}
	}
	c.Broken = broken
	return c
}
