// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import "github.com/aws/amazon-cloudwatch-agent-test/test/metric"

// IsMetricWithinBounds computes the average of the data points, and returns true if the average is within
// the input bounds (inclusive), and false otherwise.
func IsMetricWithinBounds(data metric.MetricValues, bounds metric.Bounds) bool {
	avg := computeAverage(data)
	return avg >= bounds.Lower && avg <= bounds.Upper
}

func computeAverage(data metric.MetricValues) float64 {
	var total float64 = 0
	for _, d := range data {
		total += d
	}
	if total == 0 {
		return 0
	}

	return total / float64(len(data))
}
