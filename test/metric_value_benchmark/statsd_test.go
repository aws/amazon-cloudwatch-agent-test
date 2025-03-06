// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	send_interval = 10 * time.Millisecond
)

var done = make(chan bool)

var _ test_runner.ITestRunner = (*StatsdTestRunner)(nil)

type StatsdTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *StatsdTestRunner) Validate() status.TestGroupResult {
	// Stop sender.
	close(done)
	metricsToFetch := t.GetMeasuredMetrics()
	results := make([]status.TestResult, len(metricsToFetch)*2)
	for i, metricName := range metricsToFetch {
		// First test result is for metric validation
		results[i*2] = metric.ValidateStatsdMetric(t.DimensionFactory, namespace, "InstanceId", metricName, metric.StatsdMetricValues[i], t.GetAgentRunDuration(), send_interval)
		// Second test result is for the entity validation associated with the metric
		results[i*2+1] = metric.ValidateStatsdEntity(t.DimensionFactory, metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *StatsdTestRunner) GetTestName() string {
	return "EC2StatsD"
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
		case <-done:
			return
		case <-ticker.C:
			for i, name := range metric.StatsdMetricNames {
				if strings.Contains(name, "counter") {
					// Submit twice such that the sum is metricValues[i].
					v := int64(metric.StatsdMetricValues[i])
					client.Count(name, v-500, tags, 1.0)
					client.Count(name, 500, tags, 1.0)
				} else if strings.Contains(name, "gauge") {
					// Only the most recent gauge value matters.
					client.Gauge(name, metric.StatsdMetricValues[i], tags, 1.0)
					client.Gauge(name, metric.StatsdMetricValues[i]-500, tags, 1.0)
				} else {
					v := time.Millisecond * time.Duration(metric.StatsdMetricValues[i])
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
	return metric.StatsdMetricNames
}
