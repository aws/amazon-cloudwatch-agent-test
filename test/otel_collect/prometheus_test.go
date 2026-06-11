// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	"context"
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

//go:embed resources/prometheus_scrape_config.yaml
var prometheusScrapeConfig string

const prometheusRuntime = 2 * time.Minute

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

	client, err := otelmetrics.NewClient(context.Background(), otelmetrics.TestConfig{
		Region:         t.env.Region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", t.env.Region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		SigningService: "monitoring",
	})
	if err != nil {
		testResult.Reason = fmt.Errorf("creating otel metrics client: %w", err)
		return testResult
	}

	query := fmt.Sprintf(`{__name__="%s",job="node"}`, metricName)
	results, err := client.Query(context.Background(), query)
	if err != nil {
		testResult.Reason = fmt.Errorf("querying %s: %w", metricName, err)
		return testResult
	}

	if len(results) == 0 {
		testResult.Reason = fmt.Errorf("metric %s not found", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *PrometheusOtelTestRunner) GetTestName() string                { return "OtelCollectPrometheus" }
func (t *PrometheusOtelTestRunner) GetAgentRunDuration() time.Duration { return prometheusRuntime }
func (t *PrometheusOtelTestRunner) GetAgentConfigFileName() string     { return "prometheus_config.json" }
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

	commands := []string{
		fmt.Sprintf("cat <<'EOF' | sudo tee /opt/aws/prometheus.yml\n%s\nEOF", prometheusScrapeConfig),
	}
	return common.RunCommands(commands)
}

func TestPrometheus(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &PrometheusOtelTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
