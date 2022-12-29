// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type PrometheusTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*PrometheusTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusConfig string

const prometheusMetrics = `prometheus_test_untyped{include="yes"} 1
# TYPE prometheus_test_counter counter
prometheus_test_counter{include="yes"} 1
# TYPE prometheus_test_counter_exclude counter
prometheus_test_counter_exclude{include="no"} 1
# TYPE prometheus_test_gauge gauge
prometheus_test_gauge{include="yes"} 500
# TYPE prometheus_test_summary summary
prometheus_test_summary_sum{include="yes"} 200
prometheus_test_summary_count{include="yes"} 50
prometheus_test_summary{include="yes",quantile="0"} 0.1
prometheus_test_summary{include="yes",quantile="0.5"} 0.25
prometheus_test_summary{include="yes",quantile="1"} 5.5
# TYPE prometheus_test_histogram histogram
prometheus_test_histogram_sum{include="yes"} 300
prometheus_test_histogram_count{include="yes"} 75
prometheus_test_histogram_bucket{include="yes",le="0"} 1
prometheus_test_histogram_bucket{include="yes",le="0.5"} 2
prometheus_test_histogram_bucket{include="yes",le="2.5"} 3
prometheus_test_histogram_bucket{include="yes",le="5"} 4
prometheus_test_histogram_bucket{include="yes",le="+Inf"} 5
`

func (t *PrometheusTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validatePrometheusMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *PrometheusTestRunner) getTestName() string {
	return "Prometheus"
}

func (t *PrometheusTestRunner) getAgentConfigFileName() string {
	return "prometheus_config.json"
}

func (t *PrometheusTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *PrometheusTestRunner) setupBeforeAgentRun() error {
	startPrometheusCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	return common.RunCommands(startPrometheusCommands)
}

func (t *PrometheusTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"prometheus_test_counter":       nil,
		"prometheus_test_gauge":         nil,
		"prometheus_test_summary_count": nil,
		"prometheus_test_summary_sum":   nil,
		"prometheus_test_summary":       nil,
	}
}

func (t *PrometheusTestRunner) validatePrometheusMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	fetcher, err := t.MetricFetcherFactory.GetMetricFetcher(metricName)
	if err != nil {
		return testResult
	}

	values, err := fetcher.Fetch(namespace, metricName, metric.AVERAGE)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToZero(metricName, values) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}
