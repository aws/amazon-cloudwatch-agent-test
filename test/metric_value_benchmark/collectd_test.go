// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type CollectDTestRunner struct {
	BaseTestRunner
}

var _ ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) validate() status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName := range metricsToFetch {
		testResults = append(testResults, t.validateCollectDMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDTestRunner) getTestName() string {
	return "CollectD"
}

func (t *CollectDTestRunner) getAgentConfigFileName() string {
	return "collectd_config.json"
}

func (t *CollectDTestRunner) getAgentRunDuration() time.Duration {
	return minimumAgentRuntime
}

func (t *CollectDTestRunner) setupAfterAgentRun() error {
	// EC2 Image Builder creates the collectd's default configuration and collectd will pick it up.
	// For Linux the static is at /etc/collectd.conf, fox Ubuntu it is at /etc/collectd/collectd.conf
	// Collectd's static configuration
	//		LoadPlugin network
	//		LoadPlugin cpu
	// 		<Plugin cpu>
	//			ReportByState = true
	//			ReportByCpu = true
	//			ValuesPercentage = true
	//		</Plugin>
	//		<Plugin network>
	//			Server "127.0.0.1" "25826"
	//		</Plugin>
	startCollectdCommands := []string{
		"sudo systemctl restart collectd",
	}

	return common.RunCommands(startCollectdCommands)
}

func (t *CollectDTestRunner) getMeasuredMetrics() map[string]*metric.Bounds {
	return map[string]*metric.Bounds{
		"collectd_cpu_value": nil,
	}
}

func (t *CollectDTestRunner) validateCollectDMetric(metricName string) status.TestResult {
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
