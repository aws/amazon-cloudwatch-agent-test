// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Helpers shared by the test/otel/solutions suites.

// PodLabelKServeInferenceService is KServe's InferenceService pod label as
// k8sattributes extracts it onto the resource.
const PodLabelKServeInferenceService = "k8s.pod.label.serving.kserve.io/inferenceservice"

// PipelineMatcher is a raw PromQL matcher for the cloudwatch.pipeline scope
// attribute, which the query cache does not filter on.
func PipelineMatcher(name string) string {
	return fmt.Sprintf(`,"@instrumentation.cloudwatch.pipeline"="%s"`, EscapePromQLValue(name))
}

// Series queries a metric in the cluster with extra raw PromQL matchers. Not cached.
func Series(t *testing.T, client *OtelMetricsClient, cluster, metricName, matchers string) []MetricResult {
	t.Helper()
	promql := fmt.Sprintf(`{"__name__"="%s","@resource.k8s.cluster.name"="%s"%s}`,
		EscapePromQLValue(metricName), EscapePromQLValue(cluster), matchers)
	results, err := client.Query(context.Background(), promql)
	require.NoError(t, err, "querying %s", promql)
	return results
}

// Increase returns sum(increase(metric[window])) for the matched series, so a
// suite can check that counters move rather than only that they exist.
func Increase(t *testing.T, client *OtelMetricsClient, cluster, metricName, matchers, window string) float64 {
	t.Helper()
	promql := fmt.Sprintf(`sum(increase({"__name__"="%s","@resource.k8s.cluster.name"="%s"%s}[%s]))`,
		EscapePromQLValue(metricName), EscapePromQLValue(cluster), matchers, window)
	results, err := client.Query(context.Background(), promql)
	require.NoError(t, err, "querying %s", promql)
	if len(results) == 0 {
		return 0
	}
	return results[0].Value
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
