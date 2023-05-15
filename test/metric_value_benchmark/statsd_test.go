// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	// Must match JSON config
	metrics_aggregation_interval = 30 * time.Second
	metrics_collection_interval = 5 * time.Second
	send_interval = 10 * time.Millisecond
)

var (
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
	// Stop sender.
	close(done)
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

// sender will send statsd metric values with the specified names and values.
func (t *StatsdTestRunner) sender() {
	client, _ := statsd.New(
		"127.0.0.1:8125",
		statsd.WithMaxMessagesPerPayload(1),
		statsd.WithoutTelemetry())
	defer client.Close()
	ticker := time.NewTicker(send_interval)
	defer ticker.Stop()
	tags := []string{"key:value"}
	for {
		select {
		case <- done:
			return
		case <-ticker.C:
			for i, name := range metricNames {
				if strings.Contains(name, "counter") {
					// Submit twice such that the sum is metricValues[i].
					v := int64(metricValues[i])
					client.Count(name, v - 500, tags, 1.0)
					client.Count(name, 500, tags, 1.0)
				} else if strings.Contains(name, "gauge") {
					// Only the most recent gauge value matters.
					client.Gauge(name, metricValues[i], tags, 1.0)
					client.Gauge(name, metricValues[i] - 500, tags, 1.0)
				} else {
					v := time.Millisecond * time.Duration(metricValues[i])
					v -= 100 * time.Millisecond
					client.Timing(name, v, tags, 1.0)
					v += 200 * time.Millisecond
					client.Timing(name, v, tags, 1.0)
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
	lowerBound := int(runDuration / metrics_aggregation_interval) - 2
	if len(values) < lowerBound {
		log.Printf("fail: lowerBound %v, actual %v", lowerBound, len(values))
		return testResult
	}
	// Counters get summed up over the metrics_collection_interval.
	if metricType == "counter" {
		expectedValue *= float64(metrics_collection_interval / send_interval)
	}
	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedValue)) {
		return testResult
	}
	// Check aggregation by checking sample count.
	// Expect samples to be metrics_aggregation_interval / metrics_collection_interval
	expectedSampleCount := metrics_aggregation_interval / metrics_collection_interval
	values, err = fetcher.Fetch(namespace, metricName, dims, metric.SAMPLE_COUNT,
		test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}
	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, float64(expectedSampleCount)) {
		return testResult
	}
	testResult.Status = status.SUCCESSFUL
	return testResult
}
