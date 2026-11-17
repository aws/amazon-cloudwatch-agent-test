// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// namespaceFilterConfig has the conditions the chart renders for the metrics
// namespaces include [kube-*, amazon-cloudwatch, default] and exclude
// [kube-public], next to filters that do not read the namespace.
const namespaceFilterConfig = `
processors:
  filter/cw_k8s_ci_v0_cadvisor_pod:
    metrics:
      datapoint:
        - attributes["container"] == "POD"
  filter/cw_k8s_ci_v0_scrape_metadata:
    metrics:
      metric:
        - IsMatch(name, "^(up|scrape_duration_seconds)$")
  filter/cw_k8s_ci_v0_metrics_namespaces:
    error_mode: ignore
    metrics:
      datapoint:
        - 'resource.attributes["k8s.namespace.name"] != nil and not (IsMatch(resource.attributes["k8s.namespace.name"], "^kube-") or resource.attributes["k8s.namespace.name"] == "amazon-cloudwatch" or resource.attributes["k8s.namespace.name"] == "default")'
        - 'resource.attributes["k8s.namespace.name"] != nil and (resource.attributes["k8s.namespace.name"] == "kube-public")'
  k8sattributes/cw_k8s_ci_v0_pod:
    pod_association:
      - sources: [{from: resource_attribute, name: k8s.namespace.name}]
`

func TestParseNamespaceFilter(t *testing.T) {
	f, err := ParseNamespaceFilter(namespaceFilterConfig)
	require.NoError(t, err)
	require.True(t, f.Active())
	require.Equal(t, []string{"filter/cw_k8s_ci_v0_metrics_namespaces"}, f.Processors, "only filters that read the namespace")
	for ns, dropped := range map[string]bool{
		"kube-system":       false,
		"amazon-cloudwatch": false,
		"default":           false,
		"kube-public":       true, // excluded, and exclude wins
		"monitoring":        true, // not included
		"my-kube-app":       true, // the prefix is anchored
		"default-2":         true, // an exact entry is exact
	} {
		require.Equal(t, dropped, f.Drops(ns), ns)
	}

	f, err = ParseNamespaceFilter("processors:\n  batch/x: {}\n")
	require.NoError(t, err)
	require.False(t, f.Active())
	require.False(t, f.Drops("default"), "without a namespace filter nothing is dropped")
}

func TestNamespaceFilterConditions(t *testing.T) {
	const ns = `resource.attributes["k8s.namespace.name"]`
	for cond, want := range map[string]map[string]bool{
		ns + ` != nil and (true)`:                           {"a": true, "b": true}, // exclude: ["*"]
		ns + ` == "a"`:                                      {"a": true, "b": false},
		ns + ` != "a"`:                                      {"a": false, "b": true},
		ns + ` == nil`:                                      {"a": false},
		`not ` + ns + ` == "a"`:                             {"a": false, "b": true},
		`IsMatch(` + ns + `, "^a\\.b") != true`:             {"a.b": false, "axb": true},
		ns + ` == "a" or ` + ns + ` == "b" and false`:       {"a": true, "b": false}, // and binds tighter
		`(` + ns + ` == "a" or ` + ns + ` == "b") and true`: {"a": true, "b": true, "c": false},
	} {
		n, err := parseOTTLCondition(cond)
		require.NoError(t, err, cond)
		for namespace, dropped := range want {
			require.Equal(t, dropped, n.eval(namespace).b, "%s for %s", cond, namespace)
		}
	}
}

func TestNamespaceFilterNotModeled(t *testing.T) {
	for name, cond := range map[string]string{
		"another attribute":   `resource.attributes["k8s.namespace.name"] == "a" and resource.attributes["k8s.pod.name"] == "p"`,
		"datapoint attribute": `attributes["k8s.namespace.name"] == "a"`,
		"metric name":         `resource.attributes["k8s.namespace.name"] == "a" and name == "m"`,
		"other converter":     `Concat([resource.attributes["k8s.namespace.name"]], "") == "a"`,
		"ordering":            `resource.attributes["k8s.namespace.name"] > "a"`,
		"not a boolean":       `resource.attributes["k8s.namespace.name"]`,
		"unbalanced":          `(resource.attributes["k8s.namespace.name"] == "a"`,
		"pattern not literal": `IsMatch(resource.attributes["k8s.namespace.name"], resource.attributes["k8s.namespace.name"])`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseNamespaceFilter("processors:\n  filter/x:\n    metrics:\n      metric:\n        - '" + cond + "'\n")
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrFilterNotModeled), "%v", err)
		})
	}
}
