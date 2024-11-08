// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package keu

import (
	"time"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	. "github.com/aws/amazon-cloudwatch-agent-test/test/kueue/resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	awsKueueMetricIndicator = "_neuron"
)

var expectedDimsToMetrics = map[string][]string{
    "ClusterName-ClusterQueue-Status": {
        KueuePendingWorkloads,
    },
    "ClusterName-ClusterQueue": {
        KueuePendingWorkloads,
        KueueEvictedWorkloadsTotal,
        KueueAdmittedActiveWorkloads,
        KueueClusterQueueResourceUsage,
    },
    "ClusterName-Status": {
        KueuePendingWorkloads,
    },
    "ClusterName": {
        KueuePendingWorkloads,
        KueueEvictedWorkloadsTotal,
        KueueAdmittedActiveWorkloads,
        KueueClusterQueueResourceUsage,
        KueueClusterQueueNominalQuota,
    },
    "ClusterName-Reason": {
        KueueEvictedWorkloadsTotal,
    },
    "ClusterName-ClusterQueue-Reason": {
        KueueEvictedWorkloadsTotal,
    },
    "ClusterName-ClusterQueue-Resource-Flavor": {
        KueueClusterQueueResourceUsage,
        KueueClusterQueueNominalQuota,
    },
    "ClusterName-ClusterQueue-Resource": {
        KueueClusterQueueResourceUsage,
        KueueClusterQueueNominalQuota,
    },
    "ClusterName-ClusterQueue-Flavor": {
        KueueClusterQueueResourceUsage,
        KueueClusterQueueNominalQuota,
    },
}

type AwsKueueTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*AwsKueueTestRunner)(nil)

func (t *AwsKueueTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	testResults = append(testResults, metric.ValidateMetrics(t.env, awsKueueMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	testResults = append(testResults, metric.ValidateLogsFrequency(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AwsKueueTestRunner) GetTestName() string {
	return t.testName
}

func (t *AwsKueueTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *AwsKueueTestRunner) GetAgentRunDuration() time.Duration {
	return 25 * time.Minute
}

func (t *AwsKueueTestRunner) GetMeasuredMetrics() []string {
	return nil
}
