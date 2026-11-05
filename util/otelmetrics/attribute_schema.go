// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"sort"
	"strings"
)

// KeySchema is the closed set of attribute keys one scope of a metric group
// carries. Required keys must be on every series. Expected keys may be absent
// from a series but must be on at least one series of the group, so a change
// that removes them everywhere fails. Optional keys may be absent everywhere.
// Keys under an open prefix (for example "k8s.node.label.") are allowed without
// being listed, because their names come from the cluster rather than the pipeline.
type KeySchema struct {
	Required     []string
	Expected     []string
	Optional     []string
	OpenPrefixes []string
	// Implies maps a key to keys that every series carrying it must also carry,
	// so losing part of a family that is set together fails.
	Implies map[string][]string
}

// SeriesCheck is the result of checking one scope of one series.
type SeriesCheck struct {
	Missing    []string // required keys absent
	Unexpected []string // keys the schema does not allow
	Broken     []string // "a without b" for each implication that does not hold
}

// OK reports whether the series matched the schema.
func (c SeriesCheck) OK() bool {
	return len(c.Missing) == 0 && len(c.Unexpected) == 0 && len(c.Broken) == 0
}

// Check checks one series. Keys starting with "@" are structural (the scope name
// and version) rather than attributes and are ignored.
func (s KeySchema) Check(observed map[string]string) SeriesCheck {
	var c SeriesCheck
	allowed := make(map[string]struct{}, len(s.Required)+len(s.Expected)+len(s.Optional))
	for _, k := range s.Required {
		allowed[k] = struct{}{}
		if _, ok := observed[k]; !ok {
			c.Missing = append(c.Missing, k)
		}
	}
	for _, k := range concatStrings(s.Expected, s.Optional) {
		allowed[k] = struct{}{}
	}
	for k := range observed {
		if strings.HasPrefix(k, "@") {
			continue
		}
		if _, ok := allowed[k]; ok || hasAnyPrefix(k, s.OpenPrefixes) {
			continue
		}
		c.Unexpected = append(c.Unexpected, k)
	}
	for k, implied := range s.Implies {
		if _, ok := observed[k]; !ok {
			continue
		}
		for _, want := range implied {
			if _, ok := observed[want]; !ok {
				c.Broken = append(c.Broken, k+" without "+want)
			}
		}
	}
	sort.Strings(c.Missing)
	sort.Strings(c.Unexpected)
	sort.Strings(c.Broken)
	return c
}

// Together returns implications under which the keys are all present or all absent.
func Together(keys ...string) map[string][]string {
	out := make(map[string][]string, len(keys))
	for _, k := range keys {
		for _, other := range keys {
			if other != k {
				out[k] = append(out[k], other)
			}
		}
	}
	return out
}

// MergeImplies combines implication maps.
func MergeImplies(maps ...map[string][]string) map[string][]string {
	out := map[string][]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

// GroupSchema is the attribute schema of one metric group. Datapoint scope is not
// covered: its keys come from the scraped exporter and move with its version.
type GroupSchema struct {
	Resource        KeySchema
	Instrumentation KeySchema
}

// SchemaTally counts schema problems over the series of one metric group.
type SchemaTally struct {
	schema GroupSchema
	Series int
	counts map[string]int  // series count per problem
	seen   map[string]bool // "@resource.<key>" and "@instrumentation.<key>" seen on a recorded series
}

func NewSchemaTally(schema GroupSchema) *SchemaTally {
	return &SchemaTally{schema: schema, counts: map[string]int{}, seen: map[string]bool{}}
}

// Check checks one series without recording it.
func (t *SchemaTally) Check(l MetricLabels) (resource, instrumentation SeriesCheck) {
	return t.schema.Resource.Check(l.Resource), t.schema.Instrumentation.Check(l.Instrumentation)
}

// Record adds one series and the result of Check for it.
func (t *SchemaTally) Record(l MetricLabels, resource, instrumentation SeriesCheck) {
	t.Series++
	for scope, c := range map[string]SeriesCheck{"@resource.": resource, "@instrumentation.": instrumentation} {
		for _, k := range c.Missing {
			t.counts["missing required "+scope+k]++
		}
		for _, k := range c.Unexpected {
			t.counts["undeclared "+scope+k]++
		}
		for _, b := range c.Broken {
			t.counts[scope+strings.Replace(b, " without ", " without "+scope, 1)]++
		}
	}
	for k := range l.Resource {
		t.seen["@resource."+k] = true
	}
	for k := range l.Instrumentation {
		t.seen["@instrumentation."+k] = true
	}
}

// Problems describes every problem with the number of series it affects, then
// every Expected key that no recorded series carried. It is empty when the group passed.
func (t *SchemaTally) Problems() []string {
	var out []string
	for _, p := range SortedKeys(t.counts) {
		out = append(out, fmt.Sprintf("%s on %d of %d series", p, t.counts[p], t.Series))
	}
	var unseen []string
	for scope, s := range map[string]KeySchema{"@resource.": t.schema.Resource, "@instrumentation.": t.schema.Instrumentation} {
		for _, k := range s.Expected {
			if !t.seen[scope+k] {
				unseen = append(unseen, scope+k)
			}
		}
	}
	sort.Strings(unseen)
	for _, k := range unseen {
		out = append(out, fmt.Sprintf("expected %s on at least one of %d series, found on none", k, t.Series))
	}
	return out
}

// SortedKeys returns the keys of m in order.
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func concatStrings(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
