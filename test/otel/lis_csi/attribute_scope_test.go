//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package lis_csi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestLISCSIDeprecatedSemconvAttributesRemoved asserts the lis-csi pipeline deletes
// the deprecated semantic-convention attributes the Prometheus receiver injects.
//
// url.scheme is not asserted present here, because the pipeline runs on every
// instance-store node and a node near max_total_attributes loses attributes to the
// tier cascade.
//
// The lis-csi pipeline needs no resource-scope raw-label check. It runs no
// groupbyattrs processor, so no raw Prometheus name reaches resource scope.
func TestLISCSIDeprecatedSemconvAttributesRemoved(t *testing.T) {
	t.Parallel()
	for _, metricName := range lisCsiMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available", metricName)
			require.NoError(t, otelmetrics.CheckDeprecatedSemconvRemoved(metricName, results))
		})
	}
}
