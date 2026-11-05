// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const eps = 1e-9

func TestPercentile(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{"empty", nil, 95, 0},
		{"single", []float64{5}, 95, 5},
		{"two-p50", []float64{10, 20}, 50, 15},
		{"two-p95", []float64{10, 20}, 95, 19.5},
		{"four-p0", []float64{1, 2, 3, 4}, 0, 1},
		{"four-p50", []float64{1, 2, 3, 4}, 50, 2.5},
		{"four-p90", []float64{1, 2, 3, 4}, 90, 3.7},
		{"four-p95", []float64{1, 2, 3, 4}, 95, 3.85},
		{"four-p99", []float64{1, 2, 3, 4}, 99, 3.97},
		{"four-p100", []float64{1, 2, 3, 4}, 100, 4},
		{"unsorted-input", []float64{4, 1, 3, 2}, 50, 2.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.InDelta(t, c.want, percentile(c.values, c.p), eps)
		})
	}
}

func TestExpectedShortfall(t *testing.T) {
	require.InDelta(t, 0, expectedShortfall(nil, 95), eps)
	// cutoff p50 of [1,2,3,4,5] = 3; mean of {3,4,5} = 4
	require.InDelta(t, 4, expectedShortfall([]float64{1, 2, 3, 4, 5}, 50), eps)
	// cutoff p95 of [1,2,3,4] = 3.85; only {4} qualifies
	require.InDelta(t, 4, expectedShortfall([]float64{1, 2, 3, 4}, 95), eps)
}

func TestSummaryStat(t *testing.T) {
	v := []float64{1, 2, 3, 4}
	cases := []struct {
		stat string
		want float64
	}{
		{"max", 4},
		{"p90", 3.7},
		{"p95", 3.85},
		{"p99", 3.97},
		{"es90", 4},
		{"es95", 4},
		{"average", 2.5},
		{"avg", 2.5},
		{"mean", 2.5},
		{"", 2.5},
		{" P95 ", 3.85},  // case-insensitive + trimmed
		{"garbage", 2.5}, // unknown falls back to average (current behavior)
	}
	for _, c := range cases {
		t.Run(c.stat, func(t *testing.T) {
			require.InDelta(t, c.want, summaryStat(v, c.stat), eps)
		})
	}
	require.InDelta(t, 0, summaryStat(nil, "p95"), eps)
}

func TestCalcStats(t *testing.T) {
	avg, max := calcStats([]float64{1, 2, 3, 4})
	require.InDelta(t, 2.5, avg, eps)
	require.InDelta(t, 4, max, eps)
	avg, max = calcStats(nil)
	require.InDelta(t, 0, avg, eps)
	require.InDelta(t, 0, max, eps)
}

func TestIsAllZero(t *testing.T) {
	require.True(t, isAllZero([]float64{0, 0, 0}))
	require.True(t, isAllZero(nil))
	require.True(t, isAllZero([]float64{0}))
	require.False(t, isAllZero([]float64{0, 1, 0}))
}
