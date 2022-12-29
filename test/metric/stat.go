// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

type Statistics string
type MetricValues []float64

const (
	AVERAGE Statistics = "Average"
)

// Bounds are *inclusive* upper and lower boundaries that a metric value must be within to be
// acceptable
type Bounds struct {
	Lower float64
	Upper float64
}
