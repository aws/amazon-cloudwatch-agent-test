//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package multi_efa

import (
	"sort"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// getAnyValue returns the attribute value from either resource or datapoint scope,
// preferring resource scope if set in both.
func getAnyValue(r otelmetrics.MetricResult, attr string) string {
	if v, ok := r.Labels.Resource[attr]; ok && v != "" {
		return v
	}
	return r.Labels.Datapoint[attr]
}

// filterByNodeLabel returns results where the given resource attribute equals the given value.
func filterByNodeLabel(results []otelmetrics.MetricResult, labelKey, labelValue string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if r.Labels.Resource[labelKey] == labelValue {
			out = append(out, r)
		}
	}
	return out
}

// uniqueAnyValues returns the sorted unique non-empty values of an attribute
// found in either resource or datapoint scope across all results.
func uniqueAnyValues(results []otelmetrics.MetricResult, attr string) []string {
	seen := make(map[string]struct{})
	for _, r := range results {
		if v, ok := r.Labels.Resource[attr]; ok && v != "" {
			seen[v] = struct{}{}
		} else if v, ok := r.Labels.Datapoint[attr]; ok && v != "" {
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
