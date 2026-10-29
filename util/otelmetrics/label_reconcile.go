// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AttributeDenylist is the unconditional removal configuration of an
// awsattributelimit processor, plus its attribute cap.
type AttributeDenylist struct {
	Prefixes  []string
	Keys      map[string]struct{}
	MaxTotal  int
	Processor string
}

// ParseAttributeDenylist reads the awsattributelimit processor from a rendered
// collector configuration. It returns an error if the configuration has no such
// processor or more than one, because the expectation would then be ambiguous.
func ParseAttributeDenylist(otelConfigYAML string) (AttributeDenylist, error) {
	var cfg struct {
		Processors map[string]struct {
			Prefixes []string `yaml:"unconditional_removal_prefixes"`
			Keys     []string `yaml:"unconditional_removal_keys"`
			MaxTotal int      `yaml:"max_total_attributes"`
		} `yaml:"processors"`
	}
	if err := yaml.Unmarshal([]byte(otelConfigYAML), &cfg); err != nil {
		return AttributeDenylist{}, fmt.Errorf("parsing collector configuration: %w", err)
	}
	var names []string
	for name := range cfg.Processors {
		if name == "awsattributelimit" || strings.HasPrefix(name, "awsattributelimit/") {
			names = append(names, name)
		}
	}
	if len(names) != 1 {
		return AttributeDenylist{}, fmt.Errorf("want exactly one awsattributelimit processor, found %d: %v", len(names), names)
	}
	p := cfg.Processors[names[0]]
	d := AttributeDenylist{
		Prefixes:  p.Prefixes,
		Keys:      make(map[string]struct{}, len(p.Keys)),
		MaxTotal:  p.MaxTotal,
		Processor: names[0],
	}
	for _, k := range p.Keys {
		d.Keys[k] = struct{}{}
	}
	return d, nil
}

// Denies reports whether the processor removes the resource attribute key,
// using the same exact-key-then-prefix rule as the processor itself.
func (d AttributeDenylist) Denies(key string) bool {
	if _, ok := d.Keys[key]; ok {
		return true
	}
	for _, p := range d.Prefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// AtCap reports whether a series is at the processor's attribute cap, where
// tier-based removal can explain a missing attribute. The count matches the
// processor's: resource, scope and datapoint attributes. Keys the backend
// synthesises (the @-prefixed scope name and version, and __x__ datapoint keys)
// are not attributes and are excluded.
func (d AttributeDenylist) AtCap(labels MetricLabels) bool {
	if d.MaxTotal <= 0 {
		return false
	}
	n := len(labels.Resource)
	for k := range labels.Instrumentation {
		if !strings.HasPrefix(k, "@") {
			n++
		}
	}
	for k := range labels.Datapoint {
		if !(strings.HasPrefix(k, "__") && strings.HasSuffix(k, "__")) {
			n++
		}
	}
	return n >= d.MaxTotal
}

// LabelDiff is the result of reconciling an object's Kubernetes labels with the
// resource attributes on one series.
type LabelDiff struct {
	Checked    int      // labels expected on the series
	Missing    []string // expected but absent
	Mismatched []string // present with a different value, as "key: got, want"
	Leaked     []string // removed by the extraction rules or the denylist, but present
}

// OK reports whether the series matched the object exactly.
func (l LabelDiff) OK() bool {
	return len(l.Missing) == 0 && len(l.Mismatched) == 0 && len(l.Leaked) == 0
}

// ReconcileLabels compares an object's labels with the attributes under e.Prefix
// (for example "k8s.node.label.") in resource. Every label that the extraction
// rules copy and the denylist does not match must arrive with the same value,
// and every other label must not. Empty-valued labels are skipped, because
// PromQL drops empty label values.
func ReconcileLabels(objectLabels, resource map[string]string, e LabelExtraction, d AttributeDenylist) LabelDiff {
	var diff LabelDiff
	for k, want := range objectLabels {
		attr := e.Prefix + k
		got, present := resource[attr]
		if !e.Extracts(k) || d.Denies(attr) {
			if present {
				diff.Leaked = append(diff.Leaked, k)
			}
			continue
		}
		if want == "" {
			continue
		}
		diff.Checked++
		switch {
		case !present:
			diff.Missing = append(diff.Missing, k)
		case got != want:
			diff.Mismatched = append(diff.Mismatched, fmt.Sprintf("%s: got %q, want %q", k, got, want))
		}
	}
	sort.Strings(diff.Missing)
	sort.Strings(diff.Mismatched)
	sort.Strings(diff.Leaked)
	return diff
}

// Attributes no denylist entry the chart adds may remove; a customer label exclude
// may remove the k8s.node.label.* and k8s.pod.label.* ones. The helm-charts render
// test prometheus_attribute_shape.sh holds the same list; keep the two in step.
var (
	// protectedResourceKeys and protectedPrefixes of the awsattributelimit processor;
	// unconditional removal runs before that protection, so a denylist entry would override it.
	NeverDeniedKeys = []string{
		"k8s.cluster.name", "k8s.node.name", "k8s.pod.name", "k8s.pod.uid", "k8s.namespace.name",
		"k8s.container.name", "k8s.deployment.name", "k8s.statefulset.name", "k8s.daemonset.name",
		"k8s.replicaset.name", "k8s.job.name", "k8s.cronjob.name", "k8s.workload.name",
		"k8s.workload.type", "neurondevice", "neuroncore", "efa.device", "aws.efa.eni.id",
		"volume_id", "instance_id", "k8s.pod.label.app.kubernetes.io/name",
		"k8s.pod.label.app.kubernetes.io/instance", "k8s.pod.label.app.kubernetes.io/component",
		"k8s.component.name",
		// Kept on purpose inside a label domain the denylist partly removes, so widening an entry to the domain fails.
		"k8s.node.label.kubernetes.io/os", "k8s.node.label.kubernetes.io/arch",
		"k8s.node.label.eks.amazonaws.com/nodegroup", "k8s.node.label.eks.amazonaws.com/capacityType",
		"k8s.node.label.karpenter.sh/nodepool", "k8s.node.label.karpenter.sh/capacity-type",
		"k8s.node.label.karpenter.k8s.aws/ec2nodeclass",
	}
	NeverDeniedPrefixes = []string{"cloud.", "host.", "hw."}
)

// NeverDeniedViolations lists each denylist entry that would remove a never-denied attribute.
func (d AttributeDenylist) NeverDeniedViolations() []string {
	var out []string
	for _, key := range NeverDeniedKeys {
		if _, ok := d.Keys[key]; ok {
			out = append(out, fmt.Sprintf("key %q", key))
		}
		for _, p := range d.Prefixes {
			if strings.HasPrefix(key, p) {
				out = append(out, fmt.Sprintf("prefix %q removes %q", p, key))
			}
		}
	}
	for _, protected := range NeverDeniedPrefixes {
		for key := range d.Keys {
			if strings.HasPrefix(key, protected) {
				out = append(out, fmt.Sprintf("key %q is under %q", key, protected))
			}
		}
		for _, p := range d.Prefixes {
			if strings.HasPrefix(p, protected) || strings.HasPrefix(protected, p) {
				out = append(out, fmt.Sprintf("prefix %q overlaps %q", p, protected))
			}
		}
	}
	sort.Strings(out)
	return out
}

// LabelExcludes are the filters.metrics.podLabels.exclude and nodeLabels.exclude
// patterns a cluster was deployed with: a label key, a prefix followed by "*", or
// "*" alone.
type LabelExcludes struct {
	Pod, Node []string
}

// Denylist renders the patterns as the chart does: the key k8s.pod.label.<pattern>
// or k8s.node.label.<pattern>, or a prefix without the "*" when the pattern ends in "*".
func (e LabelExcludes) Denylist() AttributeDenylist {
	d := AttributeDenylist{Keys: map[string]struct{}{}}
	for _, scope := range []struct {
		prefix   string
		patterns []string
	}{{"k8s.pod.label.", e.Pod}, {"k8s.node.label.", e.Node}} {
		for _, pattern := range scope.patterns {
			if strings.HasSuffix(pattern, "*") {
				d.Prefixes = append(d.Prefixes, scope.prefix+strings.TrimSuffix(pattern, "*"))
			} else {
				d.Keys[scope.prefix+pattern] = struct{}{}
			}
		}
	}
	return d
}

// SplitCustomerExcludes splits d into the entries that the declared customer
// excludes render to and every other entry, which the chart added.
func (d AttributeDenylist) SplitCustomerExcludes(e LabelExcludes) (chart, customer AttributeDenylist) {
	declared := e.Denylist()
	declaredPrefixes := make(map[string]struct{}, len(declared.Prefixes))
	for _, p := range declared.Prefixes {
		declaredPrefixes[p] = struct{}{}
	}
	chart = AttributeDenylist{Keys: map[string]struct{}{}, MaxTotal: d.MaxTotal, Processor: d.Processor}
	customer = AttributeDenylist{Keys: map[string]struct{}{}, MaxTotal: d.MaxTotal, Processor: d.Processor}
	for _, p := range d.Prefixes {
		if _, ok := declaredPrefixes[p]; ok {
			customer.Prefixes = append(customer.Prefixes, p)
		} else {
			chart.Prefixes = append(chart.Prefixes, p)
		}
	}
	for k := range d.Keys {
		if _, ok := declared.Keys[k]; ok {
			customer.Keys[k] = struct{}{}
		} else {
			chart.Keys[k] = struct{}{}
		}
	}
	return chart, customer
}

// Entries lists the denylist's keys and prefixes, sorted.
func (d AttributeDenylist) Entries() []string {
	out := make([]string, 0, len(d.Keys)+len(d.Prefixes))
	for k := range d.Keys {
		out = append(out, fmt.Sprintf("key %q", k))
	}
	for _, p := range d.Prefixes {
		out = append(out, fmt.Sprintf("prefix %q", p))
	}
	sort.Strings(out)
	return out
}

// ObjectReconciliation is the result of reconciling every series of a set of
// Kubernetes objects. An object passes if any one of its series reconciles,
// because a label change or a collector restart leaves an older series inside
// the query lookback window next to the current one.
type ObjectReconciliation struct {
	Objects  int                  // objects with at least one series below the cap
	Labels   int                  // labels checked, counted once per passing object
	AtCap    int                  // series skipped at the attribute cap
	Failed   map[string]LabelDiff // objects no series reconciled, with their closest series
	Required int                  // objects that must have a series
	NoSeries []string             // required objects with no series at all, sorted
}

// ReconcileObjects groups series by object and reconciles each. objectOf returns the
// object key and its Kubernetes labels, or false for a series of an unknown object.
// Every key in required must have at least one series, at the cap or below it, so
// leave out an object that a filter removes.
func ReconcileObjects(results []MetricResult, objectOf func(MetricLabels) (string, map[string]string, bool),
	required []string, e LabelExtraction, d AttributeDenylist) ObjectReconciliation {
	out := ObjectReconciliation{Failed: map[string]LabelDiff{}, Required: len(required)}
	passed := map[string]bool{}
	hasSeries := map[string]bool{}
	for _, r := range results {
		key, labels, ok := objectOf(r.Labels)
		if !ok {
			continue
		}
		hasSeries[key] = true
		if d.AtCap(r.Labels) {
			out.AtCap++
			continue
		}
		if passed[key] {
			continue
		}
		diff := ReconcileLabels(labels, r.Labels.Resource, e, d)
		if diff.OK() {
			passed[key] = true
			out.Labels += diff.Checked
			delete(out.Failed, key)
			continue
		}
		if cur, seen := out.Failed[key]; !seen || diff.problems() < cur.problems() {
			out.Failed[key] = diff
		}
	}
	out.Objects = len(passed) + len(out.Failed)
	for _, key := range required {
		if !hasSeries[key] {
			out.NoSeries = append(out.NoSeries, key)
		}
	}
	sort.Strings(out.NoSeries)
	return out
}

func (l LabelDiff) problems() int {
	return len(l.Missing) + len(l.Mismatched) + len(l.Leaked)
}
