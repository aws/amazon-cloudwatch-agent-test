// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package app_signals

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const testRetryCount = 6
const namespace = "AppSignals"

type AppSignalsMetricsRunner struct {
	test_runner.BaseTestRunner
	testName     string
	dimensionKey string
	computeType  computetype.ComputeType
}

func (t *AppSignalsMetricsRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	instructions := GetInstructionsFromTestName(t.testName, t.computeType)

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
	return "config.json"
}

func (e *AppSignalsMetricsRunner) SetupAfterAgentRun() error {
	// sends metrics data only for EC2
	if e.computeType == computetype.EC2 {
		common.RunCommand("pwd")
		cmd := `while true; export START_TIME=$(date +%s%N); do 
			cat ./resources/metrics/server_consumer.json | sed -e "s/START_TIME/$START_TIME/" > server_consumer.json;
			curl -H 'Content-Type: application/json' -d @server_consumer.json -i http://127.0.0.1:4316/v1/metrics --verbose; 
			cat ./resources/metrics/client_producer.json | sed -e "s/START_TIME/$START_TIME/" > client_producer.json; 
			curl -H 'Content-Type: application/json' -d @client_producer.json -i http://127.0.0.1:4316/v1/metrics --verbose;
			sleep 5; done`
		return common.RunAsyncCommand(cmd)
	}

	return nil
}

func GetInstructionsFromTestName(testName string, computeType computetype.ComputeType) []dimension.Instruction {
	var instructions []dimension.Instruction
	switch testName {
	case AppSignalsClientProducerTestName:
		instructions = metric.ClientProducerInstructions
	case AppSignalsServerConsumerTestName:
		instructions = metric.ServerConsumerInstructions
	default:
		return nil
	}

	if computeType == computetype.EKS {
		instructions = append(instructions, []dimension.Instruction{
			{
				Key:   "HostedIn.EKS.Cluster",
				Value: dimension.UnknownDimensionValue(),
			},
			{
				Key:   "HostedIn.K8s.Namespace",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("default")},
			},
		}...)
	} else {
		//EC2
		instructions = append(instructions, dimension.Instruction{
			Key:   "HostedIn.Environment",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("Generic")},
		})
	}

	return instructions
}

var _ test_runner.ITestRunner = (*AppSignalsMetricsRunner)(nil)
