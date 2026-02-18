// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

// DimensionSpec describes a single expected dimension.
// If Value is nil the dimension is expected to exist with any value (uses the
// dimension provider). If Value is non-nil the dimension must match exactly.
type DimensionSpec struct {
	Key   string
	Value *string // nil = unknown / any value
}

// toInstructions converts a slice of DimensionSpec into the dimension.Instruction
// slice expected by DimensionFactory.GetDimensions.
func toInstructions(specs []DimensionSpec) []dimension.Instruction {
	out := make([]dimension.Instruction, len(specs))
	for i, s := range specs {
		var v dimension.ExpectedDimensionValue
		if s.Value != nil {
			v = dimension.ExpectedDimensionValue{Value: s.Value}
		} else {
			v = dimension.UnknownDimensionValue()
		}
		out[i] = dimension.Instruction{Key: s.Key, Value: v}
	}
	return out
}

// fetchMetricValues resolves dimensions and fetches metric values from CloudWatch.
// Returns the values and whether the fetch succeeded.
func fetchMetricValues(
	base *test_runner.BaseTestRunner,
	namespace string,
	metricName string,
	specs []DimensionSpec,
) ([]float64, bool) {
	dims, failed := base.DimensionFactory.GetDimensions(toInstructions(specs))
	if len(failed) > 0 {
		log.Printf("[%s] Failed to resolve dimensions: %v", namespace, failed)
		return nil, false
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		log.Printf("[%s] Error fetching metric %s: %v", namespace, metricName, err)
		return nil, false
	}

	return values, true
}

// ValidateDimensionsPresent checks that a metric exists with the given
// dimensions and all values are >= 0.
func ValidateDimensionsPresent(
	base *test_runner.BaseTestRunner,
	namespace string,
	metricName string,
	specs []DimensionSpec,
) status.TestResult {
	result := status.TestResult{Name: metricName, Status: status.FAILED}

	values, ok := fetchMetricValues(base, namespace, metricName, specs)
	if !ok {
		return result
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		log.Printf("[%s] Expected metric %s with specified dimensions but values invalid", namespace, metricName)
		return result
	}

	result.Status = status.SUCCESSFUL
	return result
}

// ValidateDimensionsAbsent checks that a metric does NOT exist with the given
// dimensions (i.e. the dimension was dropped).
func ValidateDimensionsAbsent(
	base *test_runner.BaseTestRunner,
	namespace string,
	metricName string,
	specs []DimensionSpec,
) bool {
	values, ok := fetchMetricValues(base, namespace, metricName, specs)
	if !ok {
		return false
	}
	if len(values) != 0 {
		log.Printf("[%s] Expected dimensions to be absent for %s but found %d values", namespace, metricName, len(values))
		return false
	}
	return true
}

// ValidateGlobalAppendDimensions implements the common "global append_dimensions"
// validation pattern used by cpu, collectd, and ethtool global tests:
//  1. Verify the expected dimensions ARE present with valid values
//  2. Verify the 'host' dimension IS dropped
//
// presentSpecs are the dimensions expected to exist.
// droppedSpecs are the dimensions expected to be absent (typically including "host").
func ValidateGlobalAppendDimensions(
	base *test_runner.BaseTestRunner,
	namespace string,
	metricName string,
	presentSpecs []DimensionSpec,
	droppedSpecs []DimensionSpec,
) status.TestResult {
	result := ValidateDimensionsPresent(base, namespace, metricName, presentSpecs)
	if result.Status != status.SUCCESSFUL {
		return result
	}

	if !ValidateDimensionsAbsent(base, namespace, metricName, droppedSpecs) {
		result.Status = status.FAILED
		return result
	}

	log.Printf("[%s] Verified: dimensions present and host dropped for %s", namespace, metricName)
	return result
}

// --- Convenience dimension builders ---

// EC2Dims returns the standard InstanceId + InstanceType dimension specs.
func EC2Dims() []DimensionSpec {
	return []DimensionSpec{
		{Key: "InstanceId"},
		{Key: "InstanceType"},
	}
}

// HostDim returns a single "host" dimension spec (unknown value).
func HostDim() DimensionSpec {
	return DimensionSpec{Key: "host"}
}

// ExactDim returns a dimension spec with an exact expected value.
func ExactDim(key, value string) DimensionSpec {
	return DimensionSpec{Key: key, Value: aws.String(value)}
}
