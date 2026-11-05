//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package keda

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestKEDADeprecatedSemconvAttributesRemoved asserts the keda pipeline deletes the
// deprecated semantic-convention attributes and keeps url.scheme.
//
// The url.scheme assertion is the valuable half. keda_promote used to delete
// url.scheme, the current spelling, while keeping http.scheme, the deprecated one.
// These metrics are cluster-scoped and carry a small attribute set, so the tier
// cascade cannot explain a missing url.scheme here.
func TestKEDADeprecatedSemconvAttributesRemoved(t *testing.T) {
	t.Parallel()
	for _, metricName := range kedaMetricNames() {
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

// TestKEDARawLabelsOnlyAtDatapointScope asserts the raw Prometheus labels the keda
// groupbyattrs processor lifts to resource scope are deleted there after the
// pipeline copies them back down to datapoint scope.
func TestKEDARawLabelsOnlyAtDatapointScope(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "node"}
	for _, metricName := range kedaMetricNames() {
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
