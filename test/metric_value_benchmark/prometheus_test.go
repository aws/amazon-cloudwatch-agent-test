// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type PrometheusTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*PrometheusTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusConfig string

const prometheusMetrics = `prometheus_test_untyped{include="yes",prom_type="untyped"} 1
# TYPE prometheus_test_counter counter
prometheus_test_counter{include="yes",prom_type="counter"} 1
# TYPE prometheus_test_counter_exclude counter
prometheus_test_counter_exclude{include="no",prom_type="counter"} 1
# TYPE prometheus_test_gauge gauge
prometheus_test_gauge{include="yes",prom_type="gauge"} 500
# TYPE prometheus_test_summary summary
prometheus_test_summary_sum{include="yes",prom_type="summary"} 200
prometheus_test_summary_count{include="yes",prom_type="summary"} 50
prometheus_test_summary{include="yes",quantile="0",prom_type="summary"} 0.1
prometheus_test_summary{include="yes",quantile="0.5",prom_type="summary"} 0.25
prometheus_test_summary{include="yes",quantile="1",prom_type="summary"} 5.5
# TYPE prometheus_test_histogram histogram
prometheus_test_histogram_sum{include="yes",prom_type="histogram"} 300
prometheus_test_histogram_count{include="yes",prom_type="histogram"} 75
prometheus_test_histogram_bucket{include="yes",le="0",prom_type="histogram"} 1
prometheus_test_histogram_bucket{include="yes",le="0.5",prom_type="histogram"} 2
prometheus_test_histogram_bucket{include="yes",le="2.5",prom_type="histogram"} 3
prometheus_test_histogram_bucket{include="yes",le="5",prom_type="histogram"} 4
prometheus_test_histogram_bucket{include="yes",le="+Inf",prom_type="histogram"} 5
`

func (t *PrometheusTestRunner) Validate() status.TestGroupResult {
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

func (t *PrometheusTestRunner) GetTestName() string {
	return "Prometheus"
}

func (t *PrometheusTestRunner) GetAgentConfigFileName() string {
	return "prometheus_config.json"
}

func (t *PrometheusTestRunner) SetupBeforeAgentRun() error {
	agentConfig := test_runner.AgentConfig{
		ConfigFileName:   t.GetAgentConfigFileName(),
		SSMParameterName: t.SSMParameterName(),
		UseSSM:           t.UseSSM(),
	}
	t.SetAgentConfig(agentConfig)
	err := t.SetUpConfig()
	if err != nil {
		return err
	}
	startPrometheusCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	return common.RunCommands(startPrometheusCommands)
}

func (t *PrometheusTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_counter",
		"prometheus_test_gauge",
		//"prometheus_test_summary_count",
		//"prometheus_test_summary_sum",
		//"prometheus_test_summary",
	}
}

func (t *PrometheusTestRunner) validatePrometheusMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	var dims []types.Dimension
	var failed []dimension.Instruction

	switch metricName {
	case "prometheus_test_counter":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("counter")},
			},
		})
	case "prometheus_test_gauge":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("gauge")},
			},
		})
	case "prometheus_test_summary_count":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("summary")},
			},
		})
	case "prometheus_test_summary_sum":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("summary")},
			},
		})
	case "prometheus_test_summary":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("summary")},
			},
			{
				Key:   "quantile",
				Value: dimension.ExpectedDimensionValue{aws.String("0.5")},
			},
		})
	default:
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{})
	}

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
