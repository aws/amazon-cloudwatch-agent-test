// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type CollectDTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*CollectDTestRunner)(nil)

func (t *CollectDTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateCollectDMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *CollectDTestRunner) GetTestName() string {
	return "CollectD"
}

func (t *CollectDTestRunner) GetAgentConfigFileName() string {
	return "collectd_config.json"
}

func (t *CollectDTestRunner) SetupAfterAgentRun() error {
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

func (t *CollectDTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_cpu_value"}
}

func (t *CollectDTestRunner) validateCollectDMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "type_instance",
			Value: dimension.ExpectedDimensionValue{aws.String("user")},
		},
		{
			Key:   "instance",
			Value: dimension.ExpectedDimensionValue{aws.String("0")},
		},
		{
			Key:   "type",
			Value: dimension.ExpectedDimensionValue{aws.String("percent")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToValue(metricName, values, 1) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
