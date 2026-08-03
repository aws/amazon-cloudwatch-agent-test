//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package efa

import (
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// escapePromQL escapes backslashes and double quotes for PromQL label values.
func escapePromQL(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// getAnyValue returns the attribute value from either resource or datapoint scope,
// preferring resource scope if set in both.
func getAnyValue(r otelmetrics.MetricResult, attr string) string {
	if v, ok := r.Labels.Resource[attr]; ok && v != "" {
		return v
	}
	return r.Labels.Datapoint[attr]
}
