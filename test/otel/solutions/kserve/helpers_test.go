//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package kserve

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// series queries a metric in this cluster with extra raw PromQL matchers (for
// example on @instrumentation.cloudwatch.pipeline, which the query cache does
// not filter on). Not cached.
func series(t *testing.T, metricName, matchers string) []otelmetrics.MetricResult {
	t.Helper()
	promql := fmt.Sprintf(`{"__name__"="%s","@resource.k8s.cluster.name"="%s"%s}`,
		otelmetrics.EscapePromQLValue(metricName), otelmetrics.EscapePromQLValue(cfg.ClusterName), matchers)
	results, err := client.Query(context.Background(), promql)
	require.NoError(t, err, "querying %s", promql)
	return results
}

// pipeline is a matcher for the cloudwatch.pipeline scope attribute.
func pipeline(name string) string {
	return fmt.Sprintf(`,"@instrumentation.cloudwatch.pipeline"="%s"`, name)
}

func metricNames(defs []otelmetrics.MetricDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// requireCloudIdentity checks the cloud attributes every OTEL Container
// Insights series carries, as the KEDA and Karpenter suites do.
func requireCloudIdentity(t *testing.T, metricName string, res map[string]string) {
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
