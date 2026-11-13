// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const denylistConfig = `
processors:
  batch/x: {}
  awsattributelimit/cw_k8s_ci_v0:
    max_total_attributes: 150
    unconditional_removal_prefixes:
      - "k8s.node.label.karpenter.k8s.aws/instance-"
    unconditional_removal_keys:
      - "k8s.node.label.topology.kubernetes.io/zone"
      - "k8s.pod.label.pod-template-hash"
`

func mustDenylist(t *testing.T) AttributeDenylist {
	t.Helper()
	d, err := ParseAttributeDenylist(denylistConfig)
	require.NoError(t, err)
	return d
}

func TestParseAttributeDenylist(t *testing.T) {
	d := mustDenylist(t)
	require.Equal(t, "awsattributelimit/cw_k8s_ci_v0", d.Processor)
	require.Equal(t, 150, d.MaxTotal)
	require.Equal(t, []string{"k8s.node.label.karpenter.k8s.aws/instance-"}, d.Prefixes)
	require.Len(t, d.Keys, 2)

	_, err := ParseAttributeDenylist("processors:\n  batch/x: {}\n")
	require.Error(t, err, "no awsattributelimit processor must be an error, not an empty denylist")

	_, err = ParseAttributeDenylist("processors:\n  awsattributelimit/a: {}\n  awsattributelimit/b: {}\n")
	require.Error(t, err, "two processors make the expectation ambiguous")
}

func TestDenies(t *testing.T) {
	d := mustDenylist(t)
	require.True(t, d.Denies("k8s.node.label.topology.kubernetes.io/zone"))
	require.True(t, d.Denies("k8s.node.label.karpenter.k8s.aws/instance-family"))
	require.False(t, d.Denies("k8s.node.label.karpenter.k8s.aws/ec2nodeclass"), "the prefix must not widen to the domain")
	require.False(t, d.Denies("k8s.pod.label.topology.kubernetes.io/zone"), "exact keys are scope-specific")
}

func TestReconcileLabels(t *testing.T) {
	d := mustDenylist(t)
	node := map[string]string{
		"kubernetes.io/os":                      "linux",
		"karpenter.k8s.aws/ec2nodeclass":        "default",
		"karpenter.k8s.aws/instance-family":     "m5",
		"topology.kubernetes.io/zone":           "us-east-1a",
		"node-role.kubernetes.io/control-plane": "",
	}
	clean := map[string]string{
		"k8s.node.label.kubernetes.io/os":               "linux",
		"k8s.node.label.karpenter.k8s.aws/ec2nodeclass": "default",
	}

	t.Run("exact match", func(t *testing.T) {
		diff := ReconcileLabels(node, clean, "k8s.node.label.", d)
		require.True(t, diff.OK(), "%+v", diff)
		require.Equal(t, 2, diff.Checked, "denied and empty-valued labels are not checked")
	})

	t.Run("over-broad denylist drops a retained label", func(t *testing.T) {
		got := map[string]string{"k8s.node.label.kubernetes.io/os": "linux"}
		diff := ReconcileLabels(node, got, "k8s.node.label.", d)
		require.Equal(t, []string{"karpenter.k8s.aws/ec2nodeclass"}, diff.Missing)
	})

	t.Run("denied label still present", func(t *testing.T) {
		got := map[string]string{"k8s.node.label.karpenter.k8s.aws/instance-family": "m5"}
		for k, v := range clean {
			got[k] = v
		}
		diff := ReconcileLabels(node, got, "k8s.node.label.", d)
		require.Equal(t, []string{"karpenter.k8s.aws/instance-family"}, diff.Leaked)
	})

	t.Run("value changed in transit", func(t *testing.T) {
		got := map[string]string{"k8s.node.label.kubernetes.io/os": "windows",
			"k8s.node.label.karpenter.k8s.aws/ec2nodeclass": "default"}
		diff := ReconcileLabels(node, got, "k8s.node.label.", d)
		require.Len(t, diff.Mismatched, 1)
		require.Contains(t, diff.Mismatched[0], "kubernetes.io/os")
	})

	t.Run("labels under another prefix are ignored", func(t *testing.T) {
		got := map[string]string{"k8s.pod.label.kubernetes.io/os": "linux"}
		diff := ReconcileLabels(node, got, "k8s.node.label.", d)
		require.Len(t, diff.Missing, 2, "a pod label must not satisfy a node label")
	})
}

func TestAtCap(t *testing.T) {
	d := AttributeDenylist{MaxTotal: 3}
	l := NewMetricLabels()
	l.Resource["a"], l.Resource["b"] = "1", "2"
	l.Instrumentation["@name"] = "scope"
	l.Datapoint["__type__"] = "gauge"
	require.False(t, d.AtCap(l), "scope name and backend keys are not attributes")
	l.Datapoint["c"] = "3"
	require.True(t, d.AtCap(l))
	require.False(t, AttributeDenylist{}.AtCap(l), "no cap configured")
}

func TestNeverDeniedViolations(t *testing.T) {
	require.Empty(t, mustDenylist(t).NeverDeniedViolations())

	for name, tc := range map[string]struct {
		keys, prefixes []string
		want           []string
	}{
		"protected key":                    {keys: []string{"k8s.cluster.name"}, want: []string{`key "k8s.cluster.name"`}},
		"widened label domain":             {prefixes: []string{"k8s.node.label.kubernetes.io/"}, want: []string{`prefix "k8s.node.label.kubernetes.io/" removes "k8s.node.label.kubernetes.io/arch"`, `prefix "k8s.node.label.kubernetes.io/" removes "k8s.node.label.kubernetes.io/os"`}},
		"key under protected prefix":       {keys: []string{"cloud.availability_zone"}, want: []string{`key "cloud.availability_zone" is under "cloud."`}},
		"prefix covering protected prefix": {prefixes: []string{"ho"}, want: []string{`prefix "ho" overlaps "host."`}},
	} {
		t.Run(name, func(t *testing.T) {
			d := AttributeDenylist{Prefixes: tc.prefixes, Keys: map[string]struct{}{}}
			for _, k := range tc.keys {
				d.Keys[k] = struct{}{}
			}
			require.Equal(t, tc.want, d.NeverDeniedViolations())
		})
	}

	d := AttributeDenylist{Prefixes: []string{"k8s.node.label.karpenter.k8s.aws/instance-"}, Keys: map[string]struct{}{}}
	require.Empty(t, d.NeverDeniedViolations(), "the instance- prefix spares karpenter.k8s.aws/ec2nodeclass")
}

func TestReconcileObjects(t *testing.T) {
	d := mustDenylist(t)
	nodes := map[string]map[string]string{
		"n1": {"kubernetes.io/os": "linux", "role": "new"},
		"n2": {"kubernetes.io/os": "linux", "topology.kubernetes.io/zone": "a"},
	}
	series := func(node string, attrs map[string]string) MetricResult {
		res := map[string]string{"k8s.node.name": node}
		for k, v := range attrs {
			res["k8s.node.label."+k] = v
		}
		return MetricResult{Labels: MetricLabels{Resource: res}}
	}
	objectOf := func(l MetricLabels) (string, map[string]string, bool) {
		name := l.Resource["k8s.node.name"]
		labels, ok := nodes[name]
		return name, labels, ok
	}

	got := ReconcileObjects([]MetricResult{
		series("n1", map[string]string{"kubernetes.io/os": "linux", "role": "old"}), // before a relabel
		series("n1", map[string]string{"kubernetes.io/os": "linux", "role": "new"}),
		series("n2", map[string]string{"topology.kubernetes.io/zone": "a"}), // missing and leaked
		series("n2", map[string]string{"kubernetes.io/os": "windows"}),      // mismatched only
		series("gone", map[string]string{}),
	}, objectOf, []string{"n1", "n2"}, "k8s.node.label.", d)

	require.Equal(t, 2, got.Objects)
	require.Equal(t, 2, got.Required)
	require.Empty(t, got.NoSeries)
	require.Equal(t, 2, got.Labels, "only the passing n1 series counts")
	require.Equal(t, map[string]LabelDiff{"n2": {Checked: 1, Mismatched: []string{`kubernetes.io/os: got "windows", want "linux"`}}},
		got.Failed, "n2 keeps its closest series; the older n1 series no longer counts against it")
}

func TestReconcileObjectsRequiresASeries(t *testing.T) {
	nodes := map[string]map[string]string{"n1": {}, "n2": {}, "n3": {}, "capped": {}}
	series := func(node string, extra ...string) MetricResult {
		res := map[string]string{"k8s.node.name": node}
		for _, k := range extra {
			res[k] = "x"
		}
		return MetricResult{Labels: MetricLabels{Resource: res}}
	}
	objectOf := func(l MetricLabels) (string, map[string]string, bool) {
		name := l.Resource["k8s.node.name"]
		labels, ok := nodes[name]
		return name, labels, ok
	}

	got := ReconcileObjects([]MetricResult{
		series("n1"),
		series("capped", "a", "b"), // at the cap of 3, so not reconciled, but still a series
		series("gone"),             // an object the API no longer lists
	}, objectOf, []string{"n3", "n1", "capped", "n2"}, "k8s.node.label.", AttributeDenylist{MaxTotal: 3})

	require.Equal(t, 1, got.Objects)
	require.Equal(t, 1, got.AtCap)
	require.Equal(t, 4, got.Required)
	require.Equal(t, []string{"n2", "n3"}, got.NoSeries, "sorted, and a series at the cap counts as a series")
	require.Empty(t, got.Failed)

	got = ReconcileObjects([]MetricResult{series("n1")}, objectOf, nil, "k8s.node.label.", AttributeDenylist{})
	require.Empty(t, got.NoSeries, "with nothing required, an object without a series passes")
}
