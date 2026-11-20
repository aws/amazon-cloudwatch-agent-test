//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpudriver

import (
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// filterByPool returns only results whose node belongs to the given AKS node pool (AKS node names are
// aks-<pool>-<id>-vmss<n>). It replaces filterByHostType from test/otel/gpu: AKS nodes have no host.type.
func filterByPool(results []otelmetrics.MetricResult, pool string) []otelmetrics.MetricResult {
	var out []otelmetrics.MetricResult
	for _, r := range results {
		if strings.HasPrefix(r.Labels.Resource["k8s.node.name"], "aks-"+pool+"-") {
			out = append(out, r)
		}
	}
	return out
}
