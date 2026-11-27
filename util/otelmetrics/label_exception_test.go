// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// labelFiltersFixture reads a collector configuration from testdata/label_filters.
// Each file names the chart commit, the values and the helm template command that
// rendered it.
func labelFiltersFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "label_filters", name+".yaml"))
	require.NoError(t, err)
	return string(raw)
}

func mustFixtureDenylist(t *testing.T, cfg string) AttributeDenylist {
	t.Helper()
	d, err := ParseAttributeDenylist(cfg)
	require.NoError(t, err)
	return d
}

const (
	metricsLabelsTransform = "transform/cw_k8s_ci_v0_metrics_labels"
	gpuPrefix              = "k8s.node.label.karpenter.k8s.aws/instance-gpu-"
)

func TestParseAttributeDenylistDefaultRender(t *testing.T) {
	for _, agent := range []string{"cloudwatch-agent", "cloudwatch-agent-cluster-scraper"} {
		t.Run(agent, func(t *testing.T) {
			d := mustFixtureDenylist(t, labelFiltersFixture(t, "default--"+agent))
			require.Empty(t, d.Exceptions, "the default render has no label exception transform, and no other transform removes labels")
			require.Equal(t, "awsattributelimit/cw_k8s_ci_v0", d.Processor)
			require.Contains(t, d.Prefixes, gpuPrefix)
			require.True(t, d.Denies(gpuPrefix+"name"), "the built-in prefix removes every instance-gpu- label")
			require.False(t, d.Denies("k8s.node.label.karpenter.k8s.aws/ec2nodeclass"))
			require.Empty(t, d.NeverDeniedViolations())
		})
	}
}

func TestReconcileLabelsKeyNamedInsideBuiltinPrefix(t *testing.T) {
	// otelFilters.nodeLabels.include: ["*", "karpenter.k8s.aws/instance-gpu-name"]
	cfg := labelFiltersFixture(t, "include-key-in-builtin-prefix--cloudwatch-agent")
	d := mustFixtureDenylist(t, cfg)
	e := mustExtraction(t, cfg, "node")
	node := map[string]string{
		"kubernetes.io/arch":                   "amd64",
		"karpenter.k8s.aws/instance-gpu-name":  "a10g",
		"karpenter.k8s.aws/instance-gpu-count": "1",
	}
	// The transform keeps instance-gpu-name and removes the rest of the instance-gpu- prefix.
	delivered := map[string]string{
		"k8s.node.label.kubernetes.io/arch":                  "amd64",
		"k8s.node.label.karpenter.k8s.aws/instance-gpu-name": "a10g",
	}
	diff := ReconcileLabels(node, delivered, e, d)
	require.True(t, diff.OK(), "%+v", diff)
	require.Equal(t, 2, diff.Checked)

	delivered[gpuPrefix+"count"] = "1"
	diff = ReconcileLabels(node, delivered, e, d)
	require.Equal(t, []string{"karpenter.k8s.aws/instance-gpu-count"}, diff.Leaked)

	require.NotContains(t, d.Prefixes, gpuPrefix, "the chart moves the prefix out of the denylist")
	require.Len(t, d.Exceptions, 1, "the logs transform has the same statements, but in log_statements")
	x := d.Exceptions[0]
	require.Equal(t, metricsLabelsTransform, x.Processor)
	require.Equal(t, []string{gpuPrefix}, x.Prefixes)
	require.False(t, x.Removes(gpuPrefix+"name"))
	require.True(t, x.Removes(gpuPrefix+"name-suffix"), "a named key is kept exactly")
	require.True(t, x.Removes(gpuPrefix+"count"))
	require.False(t, x.Removes("k8s.node.label.kubernetes.io/arch"), "only the delete prefixes are removed")
	require.Empty(t, d.NeverDeniedViolations())
}

func TestReconcileLabelsPrefixNamedInsideBuiltinPrefix(t *testing.T) {
	// otelFilters.nodeLabels.include: ["*", "karpenter.k8s.aws/instance-gpu-m*", "karpenter.k8s.aws/instance-accelerator-name"]
	cfg := labelFiltersFixture(t, "include-prefix-in-builtin-prefix--cloudwatch-agent")
	d := mustFixtureDenylist(t, cfg)
	require.Len(t, d.Exceptions, 1)
	require.Equal(t, []string{gpuPrefix, "k8s.node.label.karpenter.k8s.aws/instance-accelerator-"}, d.Exceptions[0].Prefixes)

	for key, removed := range map[string]bool{
		"karpenter.k8s.aws/instance-gpu-manufacturer":   false,
		"karpenter.k8s.aws/instance-gpu-memory":         false,
		"karpenter.k8s.aws/instance-gpu-count":          true,
		"karpenter.k8s.aws/instance-gpu-name":           true,
		"karpenter.k8s.aws/instance-accelerator-name":   false,
		"karpenter.k8s.aws/instance-accelerator-count":  true,
		"karpenter.k8s.aws/instance-cpu-manufacturer":   true, // a built-in prefix the include list leaves alone
		"karpenter.k8s.aws/instance-accelerator-name-x": true,
	} {
		require.Equal(t, removed, d.Denies("k8s.node.label."+key), key)
	}

	e := mustExtraction(t, cfg, "node")
	node := map[string]string{
		"karpenter.k8s.aws/instance-gpu-manufacturer": "nvidia",
		"karpenter.k8s.aws/instance-gpu-memory":       "24576",
		"karpenter.k8s.aws/instance-gpu-count":        "1",
	}
	diff := ReconcileLabels(node, map[string]string{
		gpuPrefix + "manufacturer": "nvidia",
	}, e, d)
	require.Equal(t, []string{"karpenter.k8s.aws/instance-gpu-memory"}, diff.Missing, "a key under a kept prefix must arrive")
}

func TestReconcileLabelsExcludeRemovesKeptKey(t *testing.T) {
	// otelFilters.nodeLabels.include: ["*", "karpenter.k8s.aws/instance-gpu-m*"],
	// otelFilters.nodeLabels.exclude: ["karpenter.k8s.aws/instance-gpu-memory"]
	cfg := labelFiltersFixture(t, "exclude-kept-key--cloudwatch-agent")
	d := mustFixtureDenylist(t, cfg)
	require.Len(t, d.Exceptions, 1)
	require.False(t, d.Exceptions[0].Removes(gpuPrefix+"memory"), "the transform keeps the key")
	require.True(t, d.Denies(gpuPrefix+"memory"), "the exclude entry in the denylist removes it after the transform")
	require.False(t, d.Denies(gpuPrefix+"manufacturer"))

	e := mustExtraction(t, cfg, "node")
	node := map[string]string{
		"karpenter.k8s.aws/instance-gpu-manufacturer": "nvidia",
		"karpenter.k8s.aws/instance-gpu-memory":       "24576",
	}
	diff := ReconcileLabels(node, map[string]string{
		gpuPrefix + "manufacturer": "nvidia",
		gpuPrefix + "memory":       "24576",
	}, e, d)
	require.Equal(t, []string{"karpenter.k8s.aws/instance-gpu-memory"}, diff.Leaked)

	chart, customer := d.SplitCustomerExcludes(LabelExcludes{Node: []string{"karpenter.k8s.aws/instance-gpu-memory"}})
	require.Equal(t, d.Exceptions, chart.Exceptions, "the transform removes built-in prefixes, so it belongs to the chart")
	require.Empty(t, customer.Exceptions)
	require.Equal(t, []string{`key "` + gpuPrefix + `memory"`}, customer.Entries())
}

func TestParseAttributeDenylistChangedTransformShape(t *testing.T) {
	cfg := labelFiltersFixture(t, "include-key-in-builtin-prefix--cloudwatch-agent")
	const (
		keep  = `keep_matching_keys(cache["kept_labels"], "^(k8s\\.node\\.label\\.karpenter\\.k8s\\.aws/instance-gpu-name$)")`
		del   = `delete_matching_keys(attributes, "^k8s\\.node\\.label\\.karpenter\\.k8s\\.aws/instance-gpu-")`
		merge = `merge_maps(attributes, cache["kept_labels"], "upsert")`
	)
	// The metrics transform is the last processor in the fixture, so a change
	// after its header leaves the logs transform as rendered.
	head, metrics, found := strings.Cut(cfg, "  "+metricsLabelsTransform+":")
	require.True(t, found)
	for name, change := range map[string][2]string{
		"merge mode update":         {merge, `merge_maps(attributes, cache["kept_labels"], "update")`},
		"other cache key":           {merge, `merge_maps(attributes, cache["other"], "upsert")`},
		"keep_keys":                 {keep, `keep_keys(cache["kept_labels"], ["k8s.node.label.karpenter.k8s.aws/instance-gpu-name"])`},
		"delete pattern not prefix": {del, `delete_matching_keys(attributes, "^k8s\\.node\\.label\\..*gpu")`},
		"condition":                 {del, del + ` where attributes["k8s.node.name"] != nil`},
		"no merge":                  {"- " + merge, `- set(attributes["k8s.node.label.kept"], "true")`},
		"delete_key of a label":     {"- " + merge, "- " + merge + "\n      - delete_key(attributes, \"k8s.node.label.team\")"},
		"limit":                     {"- " + merge, "- " + merge + "\n      - limit(attributes, 100, [])"},
	} {
		t.Run(name, func(t *testing.T) {
			changed := strings.Replace(metrics, change[0], change[1], 1)
			require.NotEqual(t, metrics, changed, "the fixture no longer has %s", change[0])
			_, err := ParseAttributeDenylist(head + "  " + metricsLabelsTransform + ":" + changed)
			require.True(t, errors.Is(err, ErrFilterNotModeled), "got %v", err)
			require.Contains(t, err.Error(), metricsLabelsTransform)
		})
	}

	t.Run("other keep regex", func(t *testing.T) {
		changed := strings.Replace(metrics, keep, `keep_matching_keys(cache["kept_labels"], "^(k8s\\.node\\.label\\.karpenter\\.k8s\\.aws/instance-gpu-c)")`, 1)
		require.NotEqual(t, metrics, changed)
		d := mustFixtureDenylist(t, head+"  "+metricsLabelsTransform+":"+changed)
		require.True(t, d.Denies(gpuPrefix+"name"))
		require.False(t, d.Denies(gpuPrefix+"count"), "the model reads the keep regex, not the include list")
	})
}

func TestLabelStatementEffect(t *testing.T) {
	for _, tc := range []struct {
		context, statement string
		changesLabels      bool
	}{
		{"resource", `set(attributes["cloud.availability_zone"], attributes["k8s.node.label.topology.kubernetes.io/zone"]) where attributes["host.id"] == nil`, false},
		{"resource", `delete_key(attributes, "pod") where attributes["pod"] != nil`, false},
		{"resource", `delete_matching_keys(attributes, "^net\\.host\\.")`, false},
		{"datapoint", `delete_key(attributes, "k8s.node.label.team")`, false},
		{"datapoint", `set(resource.attributes["volume_id"], attributes["volume_id"])`, false},
		{"scope", `set(attributes["cloudwatch.pipeline"], "cadvisor")`, false},
		{"resource", `set(attributes["k8s.pod.label.team"], "a")`, true},
		{"datapoint", `set(resource.attributes["k8s.node.label.team"], "a")`, true},
		{"resource", `delete_key(attributes, "k8s.pod.label.team")`, true},
		{"resource", `delete_matching_keys(attributes, "^k8s\\.")`, true},
		{"resource", `delete_matching_keys(attributes, "team$")`, true},
		{"resource", `keep_keys(attributes, ["k8s.node.name"])`, true},
		{"resource", `merge_maps(attributes, cache["x"], "upsert")`, true},
		{"", `delete_key(resource.attributes, "k8s.node.label.team")`, true},
	} {
		got := labelStatementEffect(tc.context, tc.statement)
		require.Equal(t, tc.changesLabels, got != "", "%s: %s: %q", tc.context, tc.statement, got)
	}
}

func TestNeverDeniedViolationsLabelException(t *testing.T) {
	d := AttributeDenylist{Keys: map[string]struct{}{}, Exceptions: []LabelException{{
		Processor: "transform/x",
		Prefixes:  []string{"k8s.node.label.karpenter.sh/", "host."},
		Keep:      regexp.MustCompile(`^(k8s\.node\.label\.karpenter\.sh/nodepool$)`),
	}}}
	require.Equal(t, []string{
		`transform/x prefix "host." overlaps "host."`,
		`transform/x removes "k8s.node.label.karpenter.sh/capacity-type"`,
	}, d.NeverDeniedViolations(), "the keep regex spares karpenter.sh/nodepool")
}
