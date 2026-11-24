//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package karpenter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestKarpenterDeprecatedSemconvAttributesRemoved asserts the karpenter pipeline
// deletes the deprecated semantic-convention attributes and keeps url.scheme.
//
// The url.scheme assertion is the valuable half. karpenter_promote used to delete
// url.scheme, the current spelling, while keeping http.scheme, the deprecated one.
// These metrics are cluster-scoped and carry a small attribute set, so the tier
// cascade cannot explain a missing url.scheme here.
func TestKarpenterDeprecatedSemconvAttributesRemoved(t *testing.T) {
	t.Parallel()
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			require.NoError(t, otelmetrics.CheckDeprecatedSemconvRemoved(metricName, results))
			require.NoError(t, otelmetrics.CheckCurrentSemconvPresent(metricName, results))
		})
	}
}

// TestKarpenterRawLabelsOnlyAtDatapointScope asserts the raw Prometheus labels the
// karpenter groupbyattrs processor lifts to resource scope are deleted there after
// the pipeline copies them back down to datapoint scope.
func TestKarpenterRawLabelsOnlyAtDatapointScope(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "node"}
	for _, metricName := range karpenterMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			require.NoError(t, otelmetrics.CheckRawLabelsNotAtResourceScope(metricName, results, rawKeys))
		})
	}
}
