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
    namespacePMD = "PrometheusPMDTest21"
    epsilon      = 2.0 

    // Basic histogram values
    expectedHistogramMin  = 1.0   // Min value
    expectedHistogramMax  = 10.0  // Max value
    expectedHistogramMean = 5.5   // Mean value

    // Per-scrape values
    singleScrapeSum   = 55.0  // Sum per scrape
    singleScrapeCount = 1     // Sample count per scrape

    // Expected values for 3-minute test with 60s scrape interval
    expectedScrapes    = 3    // 180 seconds / 60 second interval 
    expectedTotalSum   = singleScrapeSum * expectedScrapes    // 55 * 3 = 165
    expectedTotalCount = singleScrapeCount * expectedScrapes  // 1 * 3 = 3

    // Tolerances
    sumTolerance     = singleScrapeSum      // Allow ±55
    countTolerance   = singleScrapeCount    // Allow ±1
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
	return 3 * time.Minute
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

	switch metricName {
	case "prometheus_test_histogram":
    if err := validateHistogramStats(statValues); err != nil {
        log.Printf("Histogram statistics validation failed: %v", err)
        return testResult
    }

    if err := validateHistogramPercentiles(metricName, dims); err != nil {
        log.Printf("Histogram percentiles validation failed: %v", err)
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
		maxValues := statValues[metric.MAXIMUM]
		log.Printf("Checking counter monotonic increase with values: %v", maxValues)
		for i := 1; i < len(maxValues); i++ {
			if maxValues[i] < maxValues[i-1] {
				log.Printf("Counter not monotonically increasing: %v", maxValues)
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
    log.Printf("Current values - Min: %v, Max: %v, Mean: %v, Sum: %v, Count: %v",
        statValues[metric.MINIMUM][0],
        statValues[metric.MAXIMUM][0],
        statValues[metric.AVERAGE][0],
        statValues[metric.SUM][0],
        statValues[metric.SAMPLE_COUNT][0])

    // Min/Max/Mean validations remain strict
    if len(statValues[metric.MINIMUM]) == 0 || !approximatelyEqual(statValues[metric.MINIMUM][0], expectedHistogramMin) {
        return fmt.Errorf("incorrect min value: got %v, want %v",
            statValues[metric.MINIMUM][0], expectedHistogramMin)
    }

    if len(statValues[metric.MAXIMUM]) == 0 || !approximatelyEqual(statValues[metric.MAXIMUM][0], expectedHistogramMax) {
        return fmt.Errorf("incorrect max value: got %v, want %v",
            statValues[metric.MAXIMUM][0], expectedHistogramMax)
    }

    if len(statValues[metric.AVERAGE]) == 0 || !approximatelyEqual(statValues[metric.AVERAGE][0], expectedHistogramMean) {
        return fmt.Errorf("mean value too far from expected: got %v, want %v ± %v",
            statValues[metric.AVERAGE][0], expectedHistogramMean, epsilon)
    }

    // Sum validation with wider tolerance
    actualSum := statValues[metric.SUM][0]
    if len(statValues[metric.SUM]) == 0 || !approximatelyEqual(actualSum, expectedTotalSum) {
        return fmt.Errorf("sum too far from expected: got %v, want %v ± %v",
            actualSum, expectedTotalSum, sumTolerance)
    }

    // Sample count validation with wider tolerance
    actualCount := statValues[metric.SAMPLE_COUNT][0]
    if len(statValues[metric.SAMPLE_COUNT]) == 0 || !approximatelyEqual(actualCount, expectedTotalCount) {
        return fmt.Errorf("sample count too far from expected: got %v, want %v ± %v",
            actualCount, expectedTotalCount, countTolerance)
    }

    // Additional logging for clarity
    log.Printf("Tolerances - Sum: ±%v, Count: ±%v, Other metrics: ±%v",
        sumTolerance, countTolerance, epsilon)
    log.Printf("Histogram statistics validation successful")
    return nil
}



func validateHistogramPercentiles(metricName string, dims []types.Dimension) error {
    log.Printf("Validating histogram percentiles...")
    fetcher := metric.MetricValueFetcher{}

    // Define percentiles to validate
    percentiles := []string{"p99", "p95", "p90", "p50"}

    // Fetch extended statistics
    values, err := fetcher.FetchExtended(
        namespacePMD,
        metricName,
        dims,
        percentiles,
        metric.HighResolutionStatPeriod,
    )
    if err != nil {
        return fmt.Errorf("failed to fetch percentiles: %v", err)
    }

    // Log all percentile values
    for p, v := range values {
        log.Printf("Percentile %s: %v", p, v)
    }

    // Validate each percentile is within expected range
    for p, v := range values {
        if len(v) == 0 {
            return fmt.Errorf("no values returned for percentile %s", p)
        }

        if v[0] < expectedHistogramMin || v[0] > expectedHistogramMax {
            return fmt.Errorf("%s outside expected range: got %v, want between %v and %v",
                p, v[0], expectedHistogramMin, expectedHistogramMax)
        }

    }

    log.Printf("Histogram percentiles validation successful")
    return nil
}


func approximatelyEqual(actual, expected float64) bool {
    diff := math.Abs(actual - expected)
    log.Printf("Comparing values - Actual: %f, Expected: %f, Difference: %f, Tolerance: %f",
        actual, expected, diff, epsilon)

    // For sum and count, use their specific tolerances
    if expected == expectedTotalSum {
        return diff <= sumTolerance
    }
    if expected == expectedTotalCount {
        return diff <= countTolerance
    }

    // For other metrics (min, max, mean), use epsilon
    return diff <= epsilon
}
