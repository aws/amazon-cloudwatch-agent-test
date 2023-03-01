// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/cactus/go-statsd-client/v5/statsd"
)

type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}
var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)

type metricInfo struct {
	metricType string
	dimensions []statsd.Tag
}
// Map the metricName to metricInfo.
var metricMap = map[string]metricInfo{
	"my_statsd_counter_1": {
		"counter",
		// Verify users can pass in dimensions via statsd.
		[]statsd.Tag{{"key1", "val1"}},
	},
	"my_statsd_gauge_2": {
		"gauge",
		[]statsd.Tag{{"key2", "val2"}, {"key3", "val3"}},
	},
}

var stopChan chan struct{} = make(chan struct{})

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateStatsdMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *StatsdTestRunner) GetTestName() string {
	return "Statsd"
}

func (t *StatsdTestRunner) GetAgentConfigFileName() string {
	return "statsd_config.json"
}

func (t *StatsdTestRunner) GetAgentRunDuration() time.Duration {
	return 5*time.Minute
}

func (t *StatsdTestRunner) SetupAfterAgentRun() error {
	// Send unique metrics each second.
	// Expect agent to collect every 5 seconds.
	// Expect agent to aggregate collections into 60 second buckets.
	go sendStatsd()
	return nil
}

func (t *StatsdTestRunner) GetMeasuredMetrics() []string {
	keys := make([]string, 0, len(metricMap))
	for k := range metricMap {
		keys = append(keys, k)
	}
	return keys
}

func (t *StatsdTestRunner) validateStatsdMetric(metricName string) status.TestResult {
	// Stop generating metrics.
	stopChan <- struct{}{}
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	// Get the metric info for the current metricName.
	// Just assume it exists.
	metricInfo := metricMap[metricName]
	// Populate the list of expected dimensions.
	instructions := []dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			// CWA adds this metric_type dimension.
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{Value: &metricInfo.metricType},
		},
	}
	//
	for _, d := range metricInfo.dimensions {
		instructions = append(instructions, dimension.Instruction{
			Key: d[0],
			Value: dimension.ExpectedDimensionValue{Value: aws.String(d[1])},
		})
	}
	dims, failed := t.DimensionFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}
	if len(values) > int(t.GetAgentRunDuration().Minutes()) {
		log.Println("fail: fetched too many metric values")
	}
	if len(values) < int(t.GetAgentRunDuration().Minutes()) {
		log.Println("fail: fetched too few metric values")
	}
	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}
	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)
	testResult.Status = status.SUCCESSFUL
	return testResult
}

// SendStatsd will run until signaled to stop.
// It sends each metric with dimensions at a 1 second interval.
func sendStatsd() error {
	config := statsd.ClientConfig{
		Address:     ":8125",
		Prefix:      "",
		UseBuffered: true,
		FlushInterval: 300 * time.Millisecond,
	}
	client, err := statsd.NewClientWithConfig(&config)
	if err != nil {
		log.Println("error creating statsd client", err)
		return err
	}
	defer client.Close()

	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-stopChan:
			return nil
		case <-ticker.C:
			// The type depends on the name.
			for name, info := range metricMap {
				switch info.metricType {
				case "counter":
					client.Inc(name, 1, 1, info.dimensions...)
				case "gauge":
					client.Gauge(name, 1, 1, info.dimensions...)
				}
			}
		}
	}
}