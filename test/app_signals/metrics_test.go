// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package app_signals

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const testRetryCount = 6
const namespace = "AWS/APM"

type AppSignalsMetricsRunner struct {
	test_runner.BaseTestRunner
	testName     string
	dimensionKey string
}

func (t *AppSignalsMetricsRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	instructions := GetInstructionsFromTestName(t.testName)

	for i, metricName := range metricsToFetch {
		var testResult status.TestResult
		for j := 0; j < testRetryCount; j++ {
			testResult = metric.ValidateAppSignalsMetric(t.DimensionFactory, namespace, metricName, instructions)
			if testResult.Status == status.SUCCESSFUL {
				break
			}
			time.Sleep(30 * time.Second)
		}
		testResults[i] = testResult
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AppSignalsMetricsRunner) GetTestName() string {
	return t.testName
}

func (t *AppSignalsMetricsRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *AppSignalsMetricsRunner) GetMeasuredMetrics() []string {
	return metric.AppSignalsMetricNames
}

func (e *AppSignalsMetricsRunner) GetAgentConfigFileName() string {
	return ""
}

func GetInstructionsFromTestName(testName string) []dimension.Instruction {
	switch testName {
	case AppSignalsClientProducerTestName:
		return metric.ClientProducerInstructions
	case AppSignalsServerConsumerTestName:
		return metric.ServerConsumerInstructions
	default:
		return nil
	}
}

var _ test_runner.ITestRunner = (*AppSignalsMetricsRunner)(nil)
