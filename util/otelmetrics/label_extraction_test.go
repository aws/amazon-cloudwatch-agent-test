// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// includeListConfig has the rules the chart renders for the include lists
// nodeLabels [kubernetes.io/*, eks.amazonaws.com/nodegroup] and podLabels
// [app.kubernetes.io/*, app], plus the labels the chart reads itself.
const includeListConfig = `
processors:
  k8sattributes/cw_k8s_ci_v0_node:
    extract:
      labels:
        - {from: node, key_regex: "(kubernetes\\.io/.*)", tag_name: "k8s.node.label.$$$1"}
        - {from: node, key: eks.amazonaws.com/nodegroup, tag_name: k8s.node.label.eks.amazonaws.com/nodegroup}
        - {from: node, key: topology.kubernetes.io/zone, tag_name: k8s.node.label.topology.kubernetes.io/zone}
      metadata: [k8s.node.name]
  k8sattributes/cw_k8s_ci_v0_pod:
    extract:
      labels:
        - {from: pod, key_regex: "(app\\.kubernetes\\.io/.*)", tag_name: "k8s.pod.label.$$$1"}
        - {from: pod, key: app, tag_name: k8s.pod.label.app}
        - {from: pod, key: component, tag_name: k8s.pod.label.component}
`

func TestParseLabelExtraction(t *testing.T) {
	t.Run("extract all", func(t *testing.T) {
		e := mustExtraction(t, extractAllConfig, "node")
		require.Equal(t, "k8s.node.label.", e.Prefix)
		require.Equal(t, "k8sattributes/cw_k8s_ci_v0_node", e.Processor)
		require.True(t, e.ExtractsAll())
		require.True(t, e.Extracts("anything/at-all"))
	})

	t.Run("include lists", func(t *testing.T) {
		node := mustExtraction(t, includeListConfig, "node")
		require.Equal(t, "k8sattributes/cw_k8s_ci_v0_node", node.Processor)
		require.False(t, node.ExtractsAll())
		require.True(t, node.Extracts("kubernetes.io/os"))
		require.True(t, node.Extracts("eks.amazonaws.com/nodegroup"))
		require.True(t, node.Extracts("topology.kubernetes.io/zone"), "the chart extracts the labels it reads")
		require.False(t, node.Extracts("node.kubernetes.io/instance-type"), "the regex is anchored at the start")
		require.False(t, node.Extracts("kubernetesXio/os"), "the dot is literal")
		require.False(t, node.Extracts("eks.amazonaws.com/nodegroup-image"), "a key rule is exact")

		pod := mustExtraction(t, includeListConfig, "pod")
		require.Equal(t, "k8s.pod.label.", pod.Prefix)
		require.Equal(t, "k8sattributes/cw_k8s_ci_v0_pod", pod.Processor)
		require.True(t, pod.Extracts("app.kubernetes.io/name"))
		require.True(t, pod.Extracts("app"))
		require.False(t, pod.Extracts("kubernetes.io/os"), "node rules do not apply to pods")
		require.False(t, pod.Extracts("app.kubernetes.io"), "the prefix needs its slash")
	})

	t.Run("a rule without from reads the pod", func(t *testing.T) {
		e := mustExtraction(t, `
processors:
  k8sattributes:
    extract:
      labels:
        - {key: app, tag_name: k8s.pod.label.app}
`, "pod")
		require.True(t, e.Extracts("app"))
	})

	for name, tc := range map[string]struct {
		cfg, from  string
		notModeled bool
	}{
		"no rules for the source": {cfg: includeListConfig, from: "namespace"},
		"no processor":            {cfg: "processors:\n  batch/x: {}\n", from: "node"},
		"rules in two processors": {cfg: `
processors:
  k8sattributes/a:
    extract: {labels: [{from: node, key: a, tag_name: k8s.node.label.a}]}
  k8sattributes/b:
    extract: {labels: [{from: node, key: b, tag_name: k8s.node.label.b}]}
`, from: "node", notModeled: true},
		"value regex": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key: a, tag_name: k8s.node.label.a, regex: "v(.*)"}]}
`, from: "node", notModeled: true},
		"renamed key": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key: a, tag_name: k8s.node.label.b}]}
`, from: "node", notModeled: true},
		"default tag name": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key: a}]}
`, from: "node", notModeled: true},
		"group inside the key": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key_regex: "kubernetes\\.io/(.*)", tag_name: "k8s.node.label.$$1"}]}
`, from: "node", notModeled: true},
		"tag name without the group": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key_regex: "(.*)", tag_name: "k8s.node.label.x"}]}
`, from: "node", notModeled: true},
		"key and key_regex": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key: a, key_regex: "(a)", tag_name: k8s.node.label.a}]}
`, from: "node"},
		"invalid key_regex": {cfg: `
processors:
  k8sattributes:
    extract: {labels: [{from: node, key_regex: "(a", tag_name: "k8s.node.label.$$$1"}]}
`, from: "node"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLabelExtraction(tc.cfg, tc.from)
			require.Error(t, err)
			require.Equal(t, tc.notModeled, errors.Is(err, ErrFilterNotModeled), "%v", err)
		})
	}
}
