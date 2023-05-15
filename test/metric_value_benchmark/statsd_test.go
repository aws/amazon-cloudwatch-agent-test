// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var (
	// metrics_collection_interval must match JSON config.
	metrics_collection_interval int = 5
	done = make(chan bool)
	metricNames = []string{
		"statsd_counter_1",
		"statsd_gauge_2",
		"statsd_timing_3",
		"statsd_counter_4",
		"statsd_gauge_5",
		"statsd_timing_6",
	}
	metricValues = []float64{1000, 2000, 3000, 4000, 5000, 6000}
)

var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)
type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	results := make([]status.TestResult, len(metricsToFetch))
	for i := range metricsToFetch {
		results[i] = t.validateStatsdMetric(metricNames[i], metricValues[i])
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *StatsdTestRunner) GetTestName() string {
	return "Statsd"
}

func (t *StatsdTestRunner) GetAgentConfigFileName() string {
	return "statsd_config.json"
}

func (t *StatsdTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *StatsdTestRunner) SetupAfterAgentRun() error {
	// Send each metric once a second.
	go t.sender()
	return nil
}

// sender will send statsd metric values with the specified names and types.
// It adds +/- variance to the metric values to test min, max, avg.
func (t *StatsdTestRunner) sender() {
	client, _ := statsd.New(
		"127.0.0.1:8125",
		statsd.WithMaxMessagesPerPayload(100),
		statsd.WithNamespace("statsd"),
		statsd.WithoutTelemetry())
	defer client.Close()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	tags := []string{"key:value"}

	for {
		select {
		case <- done:
			return
		case <-ticker.C:
			variance := float64(r.Intn(500))
			for i, name := range metricNames {
				if strings.Contains(name, "counter") {
					// Send it twice with +/- variance so the avg is predictable.
					v := int64(metricValues[i] + variance)
					client.Count(name, v, tags, 1.0)
					v = int64(metricValues[i] - variance)
					client.Count(name, v, tags, 1.0)
				} else if strings.Contains(name, "gauge") {
					v := metricValues[i] + variance
					client.Gauge(name, v, tags, 1.0)
					// Send once more with expected val
					client.Gauge(name, metricValues[i], tags, 1.0)
				} else {
					// timing
					duration := time.Millisecond * time.Duration(metricValues[i] + variance)
					client.Timing(name, duration, tags, 1.0)
					duration -= time.Duration(2 * variance)
					client.Timing(name, duration, tags, 1.0)
				}
			}
		}
	}
}

func (t *StatsdTestRunner) GetMeasuredMetrics() []string {
	return metricNames
}

func (t *StatsdTestRunner) validateStatsdMetric(
	metricName string,
	expectedValue float64,
) status.TestResult {
	// Stop sender.
	close(done)

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	instructions := []dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "key",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("value")},
		},
	}
	split := strings.Split(metricName, "_")
	metricType := split[1]
	instructions = append(instructions, dimension.Instruction{
		// CWA adds this metric_type dimension.
		Key:   "metric_type",
		Value: dimension.ExpectedDimensionValue{Value: aws.String(metricType)},
	})
	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	// Check average.
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE,
		test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	runDuration := t.GetAgentRunDuration()
	aggregationInterval := 30 * time.Second
	// If aggregation is not happening there could be a data point every 5 seconds.
	// So validate the upper bound.
	upperBound := int(runDuration/aggregationInterval) + 2
	// Allow 2 missing data points in case CW-Metrics-Web-Service has a 1 minute
	// delay to store.
	lowerBound := int(runDuration/aggregationInterval) - 2

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}
	// Counters get summed up.
	if metricType == "counter" {
		expectedValue *= 2 * float64(metrics_collection_interval)
	}
	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedValue)) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
