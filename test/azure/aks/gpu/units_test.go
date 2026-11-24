//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// otelMetricTypeLabel maps a MetricDefinition type to the value of the __type__ label that CloudWatch
// reports for it: a Prometheus gauge is a Gauge and a Prometheus counter is a Sum.
var otelMetricTypeLabel = map[string]string{
	"gauge":   "Gauge",
	"counter": "Sum",
}

// TestDCGMUnitsAndTypes asserts what the metric definitions say about each metric: its unit (when one is
// defined) and its type. The EKS GPU tests do not check either, so this is an AKS addition. The chart's
// transform/cw_k8s_ci_v0_set_unit sets the units; a metric with no unit defined here gets none.
func TestDCGMUnitsAndTypes(t *testing.T) {
	t.Parallel()
	for _, md := range managedDCGMMetrics {
		md := md
		t.Run(md.Name, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), md.Name)
			require.NoError(t, err, "querying %s", md.Name)
			require.NotEmpty(t, results, "%s not available (no GPU nodes?)", md.Name)
			wantType, known := otelMetricTypeLabel[md.MetricType]
			require.True(t, known, "%s: no __type__ mapping for metric type %q", md.Name, md.MetricType)
			for _, r := range results {
				if md.Unit != "" {
					require.Equal(t, md.Unit, r.Labels.Datapoint["__unit__"], "%s unit", md.Name)
				}
				require.Equal(t, wantType, r.Labels.Datapoint["__type__"], "%s type", md.Name)
			}
		})
	}
}
