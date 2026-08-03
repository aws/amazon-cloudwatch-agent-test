//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"sort"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// filterByHostType returns only results whose resource `host.type` matches the
// given instance type.
func filterByHostType(results []otelmetrics.MetricResult, instanceType string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if r.Labels.Resource["host.type"] == instanceType {
			out = append(out, r)
		}
	}
	return out
}

// uniqueDatapointValues returns the sorted unique non-empty values of a
// datapoint-level attribute across all results.
func uniqueDatapointValues(results []otelmetrics.MetricResult, attr string) []string {
	seen := make(map[string]struct{})
	for _, r := range results {
		if v, ok := r.Labels.Datapoint[attr]; ok && v != "" {
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
