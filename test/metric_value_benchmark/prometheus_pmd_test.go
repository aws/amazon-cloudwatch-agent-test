// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

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
	epsilon      = 2.0

	expectedHistogramMin  = 1.0
	expectedHistogramMax  = 10.0
	expectedHistogramMean = 5.2

	expectedSum   = 26.0
	expectedCount = 5

	sumTolerance   = 10.0
	countTolerance = 2.0
)

func (t *PrometheusPMDTestRunner) Validate() status.TestGroupResult {
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
	validations := []struct {
		name      string
		actual    float64
		expected  float64
		tolerance float64
	}{
		{"min", statValues[metric.MINIMUM][0], expectedHistogramMin, epsilon},
		{"max", statValues[metric.MAXIMUM][0], expectedHistogramMax, epsilon},
		{"mean", statValues[metric.AVERAGE][0], expectedHistogramMean, epsilon},
		{"sum", statValues[metric.SUM][0], expectedSum, sumTolerance},
		{"count", statValues[metric.SAMPLE_COUNT][0], expectedCount, countTolerance},
	}

	for _, v := range validations {
		if math.Abs(v.actual-v.expected) > v.tolerance {
			return fmt.Errorf("%s outside expected range: got %v, want %v ± %v",
				v.name, v.actual, v.expected, v.tolerance)
		}
	}

	return nil
}

func validateHistogramPercentiles(metricName string, dims []types.Dimension) error {
	fetcher := metric.MetricValueFetcher{}

	values, err := fetcher.FetchExtended(
		namespacePMD,
		metricName,
		dims,
		[]string{"p50", "p90", "p95", "p99"},
		metric.HighResolutionStatPeriod,
	)
	if err != nil {
		return fmt.Errorf("failed to fetch percentiles: %v", err)
	}

	// Validate percentiles are within bucket bounds
	for p, v := range values {
		if len(v) == 0 {
			return fmt.Errorf("no values returned for percentile %s", p)
		}
		if v[0] < expectedHistogramMin || v[0] > expectedHistogramMax {
			return fmt.Errorf("percentile %s outside bounds: got %v, want between %v and %v",
				p, v[0], expectedHistogramMin, expectedHistogramMax)
		}
	}

	return nil
}
