// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"sort"
	"strings"
)

// DeprecatedSemconvResourceKeys are the deprecated semantic-convention attributes
// the Prometheus receiver injects. Every Container Insights pipeline deletes them;
// their current-spelling twins (server.address, server.port, url.scheme) stay.
var DeprecatedSemconvResourceKeys = []string{"net.host.name", "net.host.port", "http.scheme"}

// CheckDeprecatedSemconvRemoved returns an error if any result carries a
// deprecated semantic-convention attribute at resource scope. Pass results from a
// pipeline that uses the Prometheus receiver.
//
// Call CheckCurrentSemconvPresent alongside this on pipelines where the inverted
// deletion is a live risk. Deleting both spellings also satisfies this check.
func CheckDeprecatedSemconvRemoved(metricName string, results []MetricResult) error {
	var problems []string
	for _, r := range results {
		for _, key := range DeprecatedSemconvResourceKeys {
			if _, present := r.Labels.Resource[key]; present {
				problems = append(problems, fmt.Sprintf("@resource.%s on node %s",
					key, r.Labels.Resource["k8s.node.name"]))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s still carries deprecated semconv attributes: %s",
		metricName, strings.Join(dedupeSorted(problems), ", "))
}

// CheckCurrentSemconvPresent returns an error if no result carries url.scheme at
// resource scope. This is the guard against an inverted deletion that removes the
// current spelling and keeps the deprecated one, which the karpenter, keda and
// kserve promote transforms each did at some point.
//
// Do not call this on a pipeline whose nodes can exceed max_total_attributes,
// because the attribute-limit processor then drops attributes by tier and
// url.scheme is not on the protected list.
func CheckCurrentSemconvPresent(metricName string, results []MetricResult) error {
	for _, r := range results {
		if r.Labels.Resource["url.scheme"] != "" {
			return nil
		}
	}
	return fmt.Errorf("%s has no url.scheme on any series: the current-spelling "+
		"attribute must survive when the deprecated http.scheme is deleted", metricName)
}

// CheckRawLabelsNotAtResourceScope returns an error if any result carries one of
// rawKeys at resource scope. Pipelines that run groupbyattrs lift these raw
// Prometheus label names to resource scope, copy them back down to datapoint
// scope for dashboard compatibility, then delete the resource-scope copies.
//
// This checks the resource side only. Assert the datapoint copies separately,
// because a pipeline that deletes both would otherwise pass.
func CheckRawLabelsNotAtResourceScope(metricName string, results []MetricResult, rawKeys []string) error {
	var problems []string
	for _, r := range results {
		for _, key := range rawKeys {
			if _, present := r.Labels.Resource[key]; present {
				problems = append(problems, fmt.Sprintf("@resource.%s on node %s",
					key, r.Labels.Resource["k8s.node.name"]))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s carries raw labels at resource scope as well as datapoint scope: %s",
		metricName, strings.Join(dedupeSorted(problems), ", "))
}

func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
