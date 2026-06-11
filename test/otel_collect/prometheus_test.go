// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus_scrape_config.yaml
var prometheusScrapeConfig string

const (
	prometheusNamespace = "CWAgent"
	prometheusRuntime   = 2 * time.Minute
)

type PrometheusOtelTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*PrometheusOtelTestRunner)(nil)

func (t *PrometheusOtelTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validatePrometheusMetric(metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *PrometheusOtelTestRunner) validatePrometheusMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "job",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("node")},
		},
	})
	if len(failed) > 0 {
		testResult.Reason = fmt.Errorf("failed to get dimensions for %s", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(prometheusNamespace, metricName, dims, metric.AVERAGE, metric.MinuteStatPeriod)
	if err != nil {
		testResult.Reason = err
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		testResult.Reason = fmt.Errorf("metric %s values not valid", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *PrometheusOtelTestRunner) GetTestName() string             { return "OtelCollectPrometheus" }
func (t *PrometheusOtelTestRunner) GetAgentRunDuration() time.Duration { return prometheusRuntime }
func (t *PrometheusOtelTestRunner) GetAgentConfigFileName() string  { return "prometheus_config.json" }
func (t *PrometheusOtelTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"node_cpu_seconds_total",
		"node_memory_MemAvailable_bytes",
		"node_filesystem_avail_bytes",
		"node_network_receive_bytes_total",
	}
}

func (t *PrometheusOtelTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	// Deploy prometheus scrape config
	commands := []string{
		fmt.Sprintf("cat <<'EOF' | sudo tee /opt/aws/prometheus.yml\n%s\nEOF", prometheusScrapeConfig),
	}
	return common.RunCommands(commands)
}

func TestPrometheus(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)

	testRunner := &PrometheusOtelTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{DimensionFactory: factory},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
