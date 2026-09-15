//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron_dra

import (
	"sort"
	"strconv"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// escapePromQL escapes a string value for safe use inside PromQL label matches.
func escapePromQL(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// filterByHostType returns results matching the given host.type.
func filterByHostType(results []otelmetrics.MetricResult, hostType string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		r := r
		if r.Labels.Resource["host.type"] == hostType {
			out = append(out, r)
		}
	}
	return out
}

// uniqueDatapointValuesList returns the sorted unique non-empty values of a
// datapoint-level attribute across all results.
func uniqueDatapointValuesList(results []otelmetrics.MetricResult, key string) []string {
	seen := make(map[string]struct{})
	for _, r := range results {
		r := r
		if v, ok := r.Labels.Datapoint[key]; ok && v != "" {
			seen[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// uniqueDatapointPairs collects distinct (a, b) pairs from two datapoint keys.
func uniqueDatapointPairs(results []otelmetrics.MetricResult, keyA, keyB string) [][2]string {
	set := make(map[[2]string]struct{})
	for _, r := range results {
		r := r
		a := r.Labels.Datapoint[keyA]
		b := r.Labels.Datapoint[keyB]
		if a == "" || b == "" {
			continue
		}
		set[[2]string{a, b}] = struct{}{}
	}
	out := make([][2]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

// isIntLike returns true if s is a non-negative integer string.
func isIntLike(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

// setKeys returns the sorted keys of a string set (for messages).
func setKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
