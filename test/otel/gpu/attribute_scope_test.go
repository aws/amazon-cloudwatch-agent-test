//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestDCGMDeprecatedSemconvAttributesRemoved asserts the dcgm pipeline deletes the
// deprecated semantic-convention attributes the Prometheus receiver injects.
//
// url.scheme is not asserted present here. GPU nodes carry enough labels to reach
// max_total_attributes, and the attribute-limit processor then drops by tier.
func TestDCGMDeprecatedSemconvAttributesRemoved(t *testing.T) {
	t.Parallel()
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			require.NoError(t, otelmetrics.CheckDeprecatedSemconvRemoved(metricName, results))
		})
	}
}

// TestDCGMRawLabelsOnlyAtDatapointScope asserts the raw Prometheus labels the dcgm
// groupbyattrs processor lifts to resource scope are deleted there after the
// pipeline copies them back down. TestDCGMHasRawPromotedKeys covers the datapoint side.
func TestDCGMRawLabelsOnlyAtDatapointScope(t *testing.T) {
	t.Parallel()
	rawKeys := []string{"pod", "namespace", "container"}
	for _, metricName := range dcgmMetricNamesList {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", metricName)
			require.NoError(t, otelmetrics.CheckRawLabelsNotAtResourceScope(metricName, results, rawKeys))
		})
	}
}
