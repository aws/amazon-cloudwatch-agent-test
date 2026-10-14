// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Helpers shared by the test/otel/solutions suites.

// PodLabelKServeInferenceService is KServe's InferenceService pod label as
// k8sattributes extracts it onto the resource.
const PodLabelKServeInferenceService = "k8s.pod.label.serving.kserve.io/inferenceservice"

// PipelineFilter is the GetWithFilter key for the cloudwatch.pipeline scope attribute.
const PipelineFilter = "@instrumentation.cloudwatch.pipeline"

// Increase returns sum(increase(metric[window])) over the series GetWithFilter
// would match, so a suite can check that counters move rather than only that
// they exist. It fails when no series match, so missing data isn't reported as
// a counter that didn't increase.
func Increase(t *testing.T, qc *QueryCache, metricName string, filters map[string]string, window string) float64 {
	t.Helper()
	promql := fmt.Sprintf("sum(increase(%s%s}[%s]))", promqlMetricSelector(metricName), qc.filterMatchers(filters), window)
	results, err := qc.client.Query(context.Background(), promql)
	require.NoError(t, err, "querying %s", promql)
	require.NotEmpty(t, results, "no %s series match %v", metricName, filters)
	return results[0].Value
}

// PublishedMetricNames lists the names of every metric with a series matching
// filters in the cluster over the last window.
func PublishedMetricNames(t *testing.T, qc *QueryCache, filters map[string]string, window time.Duration) []string {
	t.Helper()
	matcher := "{" + qc.filterMatchers(filters) + "}"
	end := time.Now()
	names, err := qc.client.MetricNames(context.Background(), matcher, end.Add(-window), end)
	require.NoError(t, err, "listing metric names for %s", matcher)
	return names
}

// RequireNoPrefix checks no name starts with any of prefixes.
func RequireNoPrefix(t *testing.T, names []string, prefixes ...string) {
	t.Helper()
	for _, n := range names {
		for _, p := range prefixes {
			require.False(t, strings.HasPrefix(n, p), "%s was published (prefix %s should be dropped)", n, p)
		}
	}
}

// MetricNames returns the names of the given metric definitions.
func MetricNames(defs []MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// RequireCloudIdentity checks the cloud attributes every OTEL Container
// Insights series carries.
func RequireCloudIdentity(t *testing.T, cfg TestConfig, metricName string, res map[string]string) {
	t.Helper()
	require.Equal(t, "aws", res["cloud.provider"], "%s cloud.provider", metricName)
	require.Equal(t, "aws_eks", res["cloud.platform"], "%s cloud.platform", metricName)
	require.Equal(t, cfg.Region, res["cloud.region"], "%s cloud.region", metricName)
	require.Equal(t, cfg.AccountID, res["cloud.account.id"], "%s cloud.account.id", metricName)
	arn := res["cloud.resource_id"]
	require.True(t, strings.HasPrefix(arn, fmt.Sprintf("arn:aws:eks:%s:", cfg.Region)) &&
		strings.HasSuffix(arn, fmt.Sprintf(":cluster/%s", cfg.ClusterName)),
		"%s cloud.resource_id %q should be this cluster's ARN", metricName, arn)
}

// RequireNoAttribute checks a key is on neither the resource nor the datapoint.
func RequireNoAttribute(t *testing.T, r MetricResult, key string) {
	t.Helper()
	_, onResource := r.Labels.Resource[key]
	_, onDatapoint := r.Labels.Datapoint[key]
	require.False(t, onResource || onDatapoint, "%s carries a %q attribute", r.MetricName, key)
}
