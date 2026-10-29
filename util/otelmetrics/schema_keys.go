// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
)

// Attribute keys the OTel Container Insights configuration sets on many metric
// groups, shared by the attribute schema rows of every suite.
var (
	// Set by the Prometheus receiver on every scrape target.
	PrometheusReceiverKeys = []string{"server.address", "server.port", "service.instance.id", "service.name", "url.scheme"}
	// Set by resourcedetection/cw_k8s_ci_v0 (eks, ec2) and set_cloud_resource_id.
	CloudKeys = []string{
		"cloud.account.id", "cloud.availability_zone", "cloud.platform", "cloud.provider",
		"cloud.region", "cloud.resource_id",
	}
	// Set by resourcedetection/cw_k8s_ci_v0 (ec2) on the node agent, and by nodemetadataenricher
	// on the cluster scraper while the node's Lease is fresh.
	HostKeys = []string{"host.id", "host.image.id", "host.name", "host.type"}
	// Set by the set_scope_* transform of every pipeline.
	ScopeKeys = []string{"cloudwatch.pipeline", "cloudwatch.solution", "cloudwatch.source"}
	// Set by k8sattributes/cw_k8s_ci_v0_pod from the pod's owner chain, so a bare pod has none; protected.
	OwnerKindKeys = []string{
		"k8s.cronjob.name", "k8s.daemonset.name", "k8s.deployment.name", "k8s.job.name",
		"k8s.replicaset.name", "k8s.statefulset.name",
	}
	// Set by set_workload from the owner kind keys; protected.
	WorkloadKeys = []string{"k8s.workload.name", "k8s.workload.type"}
)

// WorkloadImplies is set_workload's rule: any owner kind key sets both workload keys.
func WorkloadImplies() map[string][]string {
	out := Together(WorkloadKeys...)
	for _, k := range OwnerKindKeys {
		out[k] = append(out[k], WorkloadKeys...)
	}
	return out
}

// ConcatKeys joins key lists.
func ConcatKeys(groups ...[]string) []string {
	return concatStrings(groups...)
}

// Without returns keys minus the removed ones.
func Without(keys []string, removed ...string) []string {
	drop := make(map[string]struct{}, len(removed))
	for _, k := range removed {
		drop[k] = struct{}{}
	}
	var out []string
	for _, k := range keys {
		if _, ok := drop[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// Forgive may clear the problems of one series that a known transient state explains.
type Forgive func(l MetricLabels, resource SeriesCheck) SeriesCheck

// TallySchema checks every series of metrics in the cluster against schema. It reads through
// qc.Get, so it shares one cached read per metric with the suite's other every-series checks.
// qc must split no metric by host type, because that would hide the series of other instance
// types, so TallySchema fails for such a cache. It also fails if a query fails or a metric has
// no series, because an empty group would pass vacuously.
func TallySchema(ctx context.Context, qc *QueryCache, metrics []string, schema GroupSchema, forgive Forgive) (*SchemaTally, error) {
	tally := NewSchemaTally(schema)
	for _, metricName := range metrics {
		if hostTypes := qc.hostTypesFor(metricName); len(hostTypes) > 0 {
			return nil, fmt.Errorf("%s: the cache reads only host types %v, so pass a cache without host types", metricName, hostTypes)
		}
		results, err := qc.Get(ctx, metricName)
		if err != nil {
			return nil, fmt.Errorf("querying %s: %w", metricName, err)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("%s not available", metricName)
		}
		for _, r := range results {
			resource, instrumentation := tally.Check(r.Labels)
			if forgive != nil && !resource.OK() {
				resource = forgive(r.Labels, resource)
			}
			tally.Record(r.Labels, resource, instrumentation)
		}
	}
	return tally, nil
}
