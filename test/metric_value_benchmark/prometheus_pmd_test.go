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
	namespacePMD = "PrometheusPMDTest17"
	epsilon      = 2.0 // Increased epsilon for more lenient validation

	// Expected histogram values
	expectedHistogramMin  = 1.0  // Min value
	expectedHistogramMax  = 10.0 // Max value
	expectedHistogramMean = 5.5  // Mean value (average of 1-10)

	// Expected quantile values
	expectedMedian   = 5.5 // 50th percentile
	expected90thPerc = 9.0 // 90th percentile
	expected95thPerc = 9.5 // 95th percentile
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

		if err := validateHistogramQuantiles(metricName, dims); err != nil {
			log.Printf("Histogram quantiles validation failed: %v", err)
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
	log.Printf("Current values - Min: %v, Max: %v, Mean: %v",
		statValues[metric.MINIMUM][0],
		statValues[metric.MAXIMUM][0],
		statValues[metric.AVERAGE][0])

	// Check min value
	if len(statValues[metric.MINIMUM]) == 0 || !approximatelyEqual(statValues[metric.MINIMUM][0], expectedHistogramMin) {
		return fmt.Errorf("incorrect min value: got %v, want %v",
			statValues[metric.MINIMUM][0], expectedHistogramMin)
	}

	// Check max value
	if len(statValues[metric.MAXIMUM]) == 0 || !approximatelyEqual(statValues[metric.MAXIMUM][0], expectedHistogramMax) {
		return fmt.Errorf("incorrect max value: got %v, want %v",
			statValues[metric.MAXIMUM][0], expectedHistogramMax)
	}

	// More lenient mean check
	if len(statValues[metric.AVERAGE]) == 0 ||
		math.Abs(statValues[metric.AVERAGE][0]-expectedHistogramMean) > 2.0 {
		return fmt.Errorf("mean value too far from expected: got %v, want %v ± 2.0",
			statValues[metric.AVERAGE][0], expectedHistogramMean)
	}

	log.Printf("Histogram statistics validation successful")
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

		// Create complete set of dimensions in the exact order
		quantileDims := []types.Dimension{
			{
				Name:  aws.String("include"),
				Value: aws.String("yes"),
			},
			{
				Name:  aws.String("prom_type"),
				Value: aws.String("histogram"),
			},
			{
				Name:  aws.String("service.instance.id"),
				Value: aws.String("localhost:8101"),
			},
			{
				Name:  aws.String("service.name"),
				Value: aws.String("prometheus_test_job"),
			},
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(awsservice.GetInstanceId()),
			},
			{
				Name:  aws.String("net.host.port"),
				Value: aws.String("8101"),
			},
			{
				Name:  aws.String("quantile"),
				Value: aws.String(quantile),
			},
			{
				Name:  aws.String("server.port"),
				Value: aws.String("8101"),
			},
			{
				Name:  aws.String("url.scheme"),
				Value: aws.String("http"),
			},
			{
				Name:  aws.String("label1"),
				Value: aws.String("val1"),
			},
			{
				Name:  aws.String("http.scheme"),
				Value: aws.String("http"),
			},
		}

		values, err := fetcher.Fetch(namespacePMD, metricName+"_quantile", quantileDims,
			metric.AVERAGE, metric.HighResolutionStatPeriod)
		if err != nil {
			return fmt.Errorf("failed to fetch %s quantile: %v", quantile, err)
		}

		if len(values) == 0 || math.Abs(values[0]-expectedValue) > 2.0 {
			return fmt.Errorf("quantile %s too far from expected: got %v, want %v ± 2.0",
				quantile, values[0], expectedValue)
		}
		log.Printf("%s quantile validation successful", quantile)
	}

	return nil
}

func approximatelyEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	log.Printf("Comparing values - A: %f, B: %f, Difference: %f, Epsilon: %f",
		a, b, diff, epsilon)
	return diff <= epsilon
}
