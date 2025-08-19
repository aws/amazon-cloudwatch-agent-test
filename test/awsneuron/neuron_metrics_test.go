// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package awsneuron

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	. "github.com/aws/amazon-cloudwatch-agent-test/test/awsneuron/resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	awsNeuronMetricIndicator = "_neuron"
)

var expectedDimsToMetrics = map[string][]string{
	// Container level metrics
	"ClusterName,Namespace,PodName,ContainerName": {
		ContainerNeuronCoreUtil, ContainerNeuronCoreMemUsageTotal, ContainerNeuronCoreMemUsageConstants,
		ContainerNeuronCoreMemUsageModel, ContainerNeuronCoreMemUsageScratchpad, ContainerNeuronCoreMemUsageRuntime,
		ContainerNeuronCoreMemUsageTensors, ContainerNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,PodName,FullPodName,ContainerName": {
		ContainerNeuronCoreUtil, ContainerNeuronCoreMemUsageTotal, ContainerNeuronCoreMemUsageConstants,
		ContainerNeuronCoreMemUsageModel, ContainerNeuronCoreMemUsageScratchpad, ContainerNeuronCoreMemUsageRuntime,
		ContainerNeuronCoreMemUsageTensors, ContainerNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,PodName,FullPodName,ContainerName,NeuronDevice,NeuronCore": {
		ContainerNeuronCoreUtil, ContainerNeuronCoreMemUsageTotal, ContainerNeuronCoreMemUsageConstants,
		ContainerNeuronCoreMemUsageModel, ContainerNeuronCoreMemUsageScratchpad, ContainerNeuronCoreMemUsageRuntime,
		ContainerNeuronCoreMemUsageTensors,
	},
	"ClusterName,Namespace,PodName,FullPodName,ContainerName,NeuronDevice": {
		ContainerNeuronDeviceHwEccEvents,
	},
	// Pod level metrics
	"ClusterName,Namespace": {
		PodNeuronCoreUtil, PodNeuronCoreMemUsageTotal, PodNeuronCoreMemUsageConstants,
		PodNeuronCoreMemUsageModel, PodNeuronCoreMemUsageScratchpad, PodNeuronCoreMemUsageRuntime,
		PodNeuronCoreMemUsageTensors, PodNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,Service": {
		PodNeuronCoreUtil, PodNeuronCoreMemUsageTotal, PodNeuronCoreMemUsageConstants,
		PodNeuronCoreMemUsageModel, PodNeuronCoreMemUsageScratchpad, PodNeuronCoreMemUsageRuntime,
		PodNeuronCoreMemUsageTensors, PodNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,PodName": {
		PodNeuronCoreUtil, PodNeuronCoreMemUsageTotal, PodNeuronCoreMemUsageConstants,
		PodNeuronCoreMemUsageModel, PodNeuronCoreMemUsageScratchpad, PodNeuronCoreMemUsageRuntime,
		PodNeuronCoreMemUsageTensors, PodNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,PodName,FullPodName": {
		PodNeuronCoreUtil, PodNeuronCoreMemUsageTotal, PodNeuronCoreMemUsageConstants,
		PodNeuronCoreMemUsageModel, PodNeuronCoreMemUsageScratchpad, PodNeuronCoreMemUsageRuntime,
		PodNeuronCoreMemUsageTensors, PodNeuronDeviceHwEccEvents,
	},
	"ClusterName,Namespace,PodName,FullPodName,NeuronDevice,NeuronCore": {
		PodNeuronCoreUtil, PodNeuronCoreMemUsageTotal, PodNeuronCoreMemUsageConstants,
		PodNeuronCoreMemUsageModel, PodNeuronCoreMemUsageScratchpad, PodNeuronCoreMemUsageRuntime,
		PodNeuronCoreMemUsageTensors,
	},
	"ClusterName,Namespace,PodName,FullPodName,NeuronDevice": {
		PodNeuronDeviceHwEccEvents,
	},
	// Node level metrics
	"ClusterName": {
		NodeNeuronCoreUtil, NodeNeuronCoreMemUsageTotal, NodeNeuronCoreMemUsageConstants,
		NodeNeuronCoreMemUsageModel, NodeNeuronCoreMemUsageScratchpad, NodeNeuronCoreMemUsageRuntime,
		NodeNeuronCoreMemUsageTensors, NodeExecutionErrorsTotal, NodeNeuronDeviceRuntimeMemoryUsed,
		NodeNeuronExecutionLatency, NodeNeuronDeviceHwEccEvents,
	},
	"ClusterName,UltraServer": {
		NodeNeuronCoreUtil, NodeNeuronCoreMemUsageTotal, NodeNeuronCoreMemUsageConstants,
		NodeNeuronCoreMemUsageModel, NodeNeuronCoreMemUsageScratchpad, NodeNeuronCoreMemUsageRuntime,
		NodeNeuronCoreMemUsageTensors, NodeExecutionErrorsTotal, NodeNeuronDeviceRuntimeMemoryUsed,
		NodeNeuronExecutionLatency, NodeNeuronDeviceHwEccEvents,
	},
	"ClusterName,InstanceId,NodeName": {
		NodeExecutionErrorsTotal, NodeNeuronDeviceRuntimeMemoryUsed, NodeNeuronExecutionLatency,
		NodeNeuronDeviceHwEccEvents,
	},
	"ClusterName,InstanceType,InstanceId,NodeName,NeuronDevice,NeuronCore": {
		NodeNeuronCoreUtil, NodeNeuronCoreMemUsageTotal, NodeNeuronCoreMemUsageConstants,
		NodeNeuronCoreMemUsageModel, NodeNeuronCoreMemUsageScratchpad, NodeNeuronCoreMemUsageRuntime,
		NodeNeuronCoreMemUsageTensors,
	},
	"ClusterName,InstanceId,NodeName,NeuronDevice": {
		NodeNeuronDeviceHwEccEvents,
	},
}

type AwsNeuronTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*AwsNeuronTestRunner)(nil)

func (t *AwsNeuronTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, awsNeuronMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	testResults = append(testResults, metric.ValidateLogsFrequency(t.env))
	testResults = append(testResults, metric.ValidateNeuronCoreUtilizationValuesLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AwsNeuronTestRunner) GetTestName() string {
	return t.testName
}

func (t *AwsNeuronTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *AwsNeuronTestRunner) GetAgentRunDuration() time.Duration {
	return 25 * time.Minute
}

func (t *AwsNeuronTestRunner) GetMeasuredMetrics() []string {
	return nil
}
