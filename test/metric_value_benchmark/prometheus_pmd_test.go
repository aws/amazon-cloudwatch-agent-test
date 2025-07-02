// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"log"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type PrometheusPMDTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*PrometheusPMDTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusPMDConfig string

const (
	namespacePMD = "PrometheusPMDTest"
	epsilon      = 0.1

	// Expected histogram values (matching generator's histogramValues)
	expectedHistogramMin  = 1.0  // Matches first value in generator
	expectedHistogramMax  = 9.0  // Matches last value in generator
	expectedHistogramMean = 5.0  // Average of all values
	expectedSampleCount   = 15.0 // Length of histogramValues in generator
	expectedHistogramSum  = 75.0 // Precalculated sum of histogramValues

	// Expected quantile values (calculated from generator's fixed values)
	expectedMedian   = 5.0 // Middle value (8th element)
	expected90thPerc = 8.0 // 14th element (0.90 * 15)
	expected95thPerc = 8.5 // 14th element (0.95 * 15)

	// Retry configuration
	maxRetries = 3
	retryDelay = 10 * time.Second
)

func (t *PrometheusPMDTestRunner) Validate() status.TestGroupResult {
	log.Printf("Starting PMD test validation")
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validatePMDMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *PrometheusPMDTestRunner) GetTestName() string {
	return "PrometheusPMDTest"
}

func (t *PrometheusPMDTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute // Increased to ensure enough samples
}

func (t *PrometheusPMDTestRunner) GetAgentConfigFileName() string {
	return "prometheus_pmd_config.json"
}

func (t *PrometheusPMDTestRunner) SetupBeforeAgentRun() error {
	log.Printf("Setting up PMD test")
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	setupCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus.yaml\n%s\nEOF", prometheusPMDConfig),
	}

	err = common.RunCommands(setupCommands)
	if err != nil {
		return err
	}

	err = t.startPrometheusGenerator()
	if err != nil {
		return fmt.Errorf("failed to start prometheus generator: %v", err)
	}

	log.Printf("Waiting for generator to initialize")
	time.Sleep(5 * time.Second)

	return nil
}

func (t *PrometheusPMDTestRunner) startPrometheusGenerator() error {
	cmd := "go run ../../cmd/prometheus-generator -port=8101"
	return common.RunAsyncCommand(cmd)
}

func (t *PrometheusPMDTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_untyped",
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_histogram",
	}
}

func (t *PrometheusPMDTestRunner) validatePMDMetric(metricName string) status.TestResult {
	log.Printf("Validating metric: %s", metricName)

	for retry := 0; retry < maxRetries; retry++ {
		testResult := t.validateMetricWithRetry(metricName)
		if testResult.Status == status.SUCCESSFUL {
			log.Printf("Metric %s validation successful", metricName)
			return testResult
		}
		log.Printf("Retry %d failed for metric %s, waiting %v before next attempt",
			retry+1, metricName, retryDelay)
		time.Sleep(retryDelay)
	}

	log.Printf("Metric %s validation failed after %d retries", metricName, maxRetries)
	return status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
}

func (t *PrometheusPMDTestRunner) validateMetricWithRetry(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.ExpectedDimensionValue{aws.String(awsservice.GetInstanceId())},
		},
	})

	if len(failed) > 0 {
		log.Printf("Failed to get dimensions for metric %s", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	stats := []metric.Statistics{metric.AVERAGE, metric.MAXIMUM, metric.MINIMUM, metric.SUM, metric.SAMPLE_COUNT}
	statValues := make(map[metric.Statistics][]float64)

	for _, stat := range stats {
		values, err := fetcher.Fetch(namespacePMD, metricName, dims, stat, metric.HighResolutionStatPeriod)
		if err != nil {
			log.Printf("Failed to fetch %s for metric %s: %v", stat, metricName, err)
			return testResult
		}
		statValues[stat] = values
		log.Printf("Fetched %s values for %s: %v", stat, metricName, values)
	}

	// Validate based on metric type
	switch metricName {
	case "prometheus_test_histogram":
		if err := validateHistogramStats(statValues); err != nil {
			log.Printf("Histogram statistics validation failed: %v", err)
			return testResult
		}

		if err := validateHistogramSampleCount(statValues); err != nil {
			log.Printf("Histogram sample count validation failed: %v", err)
			return testResult
		}

		if err := validateHistogramSum(statValues); err != nil {
			log.Printf("Histogram sum validation failed: %v", err)
			return testResult
		}

		if err := validateHistogramQuantiles(metricName, dims); err != nil {
			log.Printf("Histogram quantiles validation failed: %v", err)
			return testResult
		}

		if err := validateHistogramStability(statValues); err != nil {
			log.Printf("Histogram stability validation failed: %v", err)
			return testResult
		}

	case "prometheus_test_gauge":
		if len(statValues[metric.MAXIMUM]) > 0 && statValues[metric.MAXIMUM][0] > 1000 {
			log.Printf("Gauge max value too high: %v", statValues[metric.MAXIMUM][0])
			return testResult
		}
		if len(statValues[metric.MINIMUM]) > 0 && statValues[metric.MINIMUM][0] < 0 {
			log.Printf("Gauge min value negative: %v", statValues[metric.MINIMUM][0])
			return testResult
		}

	case "prometheus_test_counter":
		avgValues := statValues[metric.AVERAGE]
		for i := 1; i < len(avgValues); i++ {
			if avgValues[i] < avgValues[i-1] {
				log.Printf("Counter not monotonically increasing: %v", avgValues)
				return testResult
			}
		}

	case "prometheus_test_untyped":
		if len(statValues[metric.MAXIMUM]) > 0 && statValues[metric.MAXIMUM][0] > 100 {
			log.Printf("Untyped max value too high: %v", statValues[metric.MAXIMUM][0])
			return testResult
		}
		if len(statValues[metric.MINIMUM]) > 0 && statValues[metric.MINIMUM][0] < 1 {
			log.Printf("Untyped min value too low: %v", statValues[metric.MINIMUM][0])
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func validateHistogramStats(statValues map[metric.Statistics][]float64) error {
	log.Printf("Validating histogram statistics...")
	log.Printf("Current values - Min: %v, Max: %v, Mean: %v",
		statValues[metric.MINIMUM][0],
		statValues[metric.MAXIMUM][0],
		statValues[metric.AVERAGE][0])

	if len(statValues[metric.MINIMUM]) == 0 || !approximatelyEqual(statValues[metric.MINIMUM][0], expectedHistogramMin) {
		return fmt.Errorf("incorrect min value: got %v, want %v",
			statValues[metric.MINIMUM][0], expectedHistogramMin)
	}

	if len(statValues[metric.MAXIMUM]) == 0 || !approximatelyEqual(statValues[metric.MAXIMUM][0], expectedHistogramMax) {
		return fmt.Errorf("incorrect max value: got %v, want %v",
			statValues[metric.MAXIMUM][0], expectedHistogramMax)
	}

	if len(statValues[metric.AVERAGE]) == 0 || !approximatelyEqual(statValues[metric.AVERAGE][0], expectedHistogramMean) {
		return fmt.Errorf("incorrect mean value: got %v, want %v",
			statValues[metric.AVERAGE][0], expectedHistogramMean)
	}

	log.Printf("Histogram statistics validation successful")
	return nil
}

func validateHistogramSampleCount(statValues map[metric.Statistics][]float64) error {
	log.Printf("Validating histogram sample count...")
	if len(statValues[metric.SAMPLE_COUNT]) == 0 || !approximatelyEqual(statValues[metric.SAMPLE_COUNT][0], expectedSampleCount) {
		return fmt.Errorf("incorrect sample count: got %v, want %v",
			statValues[metric.SAMPLE_COUNT][0], expectedSampleCount)
	}
	log.Printf("Sample count validation successful")
	return nil
}

func validateHistogramSum(statValues map[metric.Statistics][]float64) error {
	log.Printf("Validating histogram sum...")
	if len(statValues[metric.SUM]) == 0 || !approximatelyEqual(statValues[metric.SUM][0], expectedHistogramSum) {
		return fmt.Errorf("incorrect sum: got %v, want %v",
			statValues[metric.SUM][0], expectedHistogramSum)
	}
	log.Printf("Sum validation successful")
	return nil
}

func validateHistogramQuantiles(metricName string, dims []types.Dimension) error {
	log.Printf("Validating histogram quantiles...")
	fetcher := metric.MetricValueFetcher{}

	quantiles := map[string]float64{
		"0.50": expectedMedian,
		"0.90": expected90thPerc,
		"0.95": expected95thPerc,
	}

	for quantile, expectedValue := range quantiles {
		log.Printf("Validating %s quantile...", quantile)
		quantileDims := append(dims, types.Dimension{
			Name:  aws.String("quantile"),
			Value: aws.String(quantile),
		})

		values, err := fetcher.Fetch(namespacePMD, metricName+"_quantile", quantileDims,
			metric.AVERAGE, metric.HighResolutionStatPeriod)
		if err != nil {
			return fmt.Errorf("failed to fetch %s quantile: %v", quantile, err)
		}

		if len(values) == 0 || !approximatelyEqual(values[0], expectedValue) {
			return fmt.Errorf("incorrect %s quantile: got %v, want %v",
				quantile, values[0], expectedValue)
		}
		log.Printf("%s quantile validation successful", quantile)
	}

	return nil
}

func validateHistogramStability(statValues map[metric.Statistics][]float64) error {
	log.Printf("Validating histogram stability...")
	if len(statValues[metric.AVERAGE]) < 3 {
		return fmt.Errorf("insufficient data points for stability check")
	}

	lastValues := statValues[metric.AVERAGE][len(statValues[metric.AVERAGE])-3:]
	for i := 1; i < len(lastValues); i++ {
		if !approximatelyEqual(lastValues[i], lastValues[i-1]) {
			return fmt.Errorf("values haven't stabilized yet: %v", lastValues)
		}
	}
	log.Printf("Stability validation successful")
	return nil
}

func approximatelyEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	log.Printf("Comparing values - A: %f, B: %f, Difference: %f, Epsilon: %f",
		a, b, diff, epsilon)
	return diff <= epsilon
}
