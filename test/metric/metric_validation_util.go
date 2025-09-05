// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

import (
	"fmt"
	"log"
	"math"
)

var CpuMetrics = []string{"cpu_time_active", "cpu_time_guest", "cpu_time_guest_nice", "cpu_time_idle", "cpu_time_iowait", "cpu_time_irq",
	"cpu_time_nice", "cpu_time_softirq", "cpu_time_steal", "cpu_time_system", "cpu_time_user",
	"cpu_usage_active", "cpu_usage_guest", "cpu_usage_guest_nice", "cpu_usage_idle", "cpu_usage_iowait",
	"cpu_usage_irq", "cpu_usage_nice", "cpu_usage_softirq", "cpu_usage_steal", "cpu_usage_system", "cpu_usage_user", "cpu_load_average"}

// IsAllValuesGreaterThanOrEqualToExpectedValue will compare if the given array is larger than 0
// and check if the average value for the array is not la
// https://github.com/aws/amazon-cloudwatch-agent-test/pull/162
func IsAllValuesGreaterThanOrEqualToExpectedValueWithError(metricName string, values []float64, expectedValue float64) error {
	if len(values) == 0 {
		err := fmt.Errorf("No values found %v", metricName)
		log.Println(err)
		return err
	}

	totalSum := 0.0
	for _, value := range values {
		if value < 0 && expectedValue >= 0 {
			err := fmt.Errorf("Values are not all greater than or equal to zero for %s", metricName)
			log.Println(err)
			return err
		}
		totalSum += value
	}
	metricErrorBound := 0.15
	metricAverageValue := totalSum / float64(len(values))
	upperBoundValue := expectedValue * (1 + metricErrorBound)
	lowerBoundValue := expectedValue * (1 - metricErrorBound)
	if expectedValue > 0 && (metricAverageValue > upperBoundValue || metricAverageValue < lowerBoundValue) {
		err := fmt.Errorf("The average value %f for metric %s are not within bound [%f, %f]",
			metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
		log.Println(err)
		return err
	}

	log.Printf("The average value %f for metric %s are within bound [%f, %f]",
		metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
	return nil
}

// IsAllValuesGreaterThanOrEqualToExpectedValue will compare if the given array is larger than 0
// and check if the average value for the array is not la
// https://github.com/aws/amazon-cloudwatch-agent-test/pull/162
func IsAllValuesGreaterThanOrEqualToExpectedValue(metricName string, values []float64, expectedValue float64) bool {
	err := IsAllValuesGreaterThanOrEqualToExpectedValueWithError(metricName, values, expectedValue)
	return err == nil
}

func FloatEqualWithEpsilon(a, b, epsilon float64) bool {
	// Handle special cases like NaN and Inf
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return a == b
	}

	// Compare using relative error
	diff := math.Abs(a - b)
	if a == b {
		return true
	} else if a == 0 || b == 0 || diff < math.SmallestNonzeroFloat64 {
		// Use absolute error if one of the numbers is zero
		return diff < epsilon
	}
	// Use relative error
	return diff/(math.Abs(a)+math.Abs(b)) < epsilon
}

func FloatEqual(a, b float64) bool {
	return FloatEqualWithEpsilon(a, b, float64(1e-10))
}
