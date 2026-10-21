// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"math"
	"sort"
	"strings"
)

// isAllZero reports whether every value in the series is zero. An all-zero
// series means no data was collected for that pod in the window; scoring it
// would drag the stats artificially low, so callers skip these series.
func isAllZero(values []float64) bool {
	for _, v := range values {
		if v != 0 {
			return false
		}
	}
	return true
}

// calcStats computes the average and maximum from a series of data points.
// performance_test.go uses the average and regression_test.go uses the max.
func calcStats(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum, max float64
	for _, v := range values {
		sum += v
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(values))
	return avg, max
}

// summaryStat reduces a series to a single value using the named statistic
// (max, p90, p95, p99, es90, es95, or average). Driven by the "stat" field in the config.
func summaryStat(values []float64, stat string) float64 {
	if len(values) == 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(stat)) {
	case "max":
		_, max := calcStats(values)
		return max
	case "p90":
		return percentile(values, 90)
	case "p95":
		return percentile(values, 95)
	case "p99":
		return percentile(values, 99)
	case "es90":
		return expectedShortfall(values, 90)
	case "es95":
		return expectedShortfall(values, 95)
	case "average", "avg", "mean", "":
		avg, _ := calcStats(values)
		return avg
	default:
		avg, _ := calcStats(values)
		return avg
	}
}

// expectedShortfall returns the mean of the samples at or above the p-th
// percentile (CVaR / tail-conditional mean) — a smoothed view of the worst tail
// that is steadier than p95 but still moves when the tail genuinely shifts.
func expectedShortfall(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	cutoff := percentile(values, p)
	var sum float64
	var count int
	for _, v := range sorted {
		if v >= cutoff {
			sum += v
			count++
		}
	}
	if count == 0 {
		return cutoff
	}
	return sum / float64(count)
}

// percentile returns the linearly-interpolated p-th percentile (0-100) of values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
