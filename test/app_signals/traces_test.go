// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package app_signals

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	lookbackDuration     = time.Duration(-5) * time.Minute
	EKSClusterAnnotation = "HostedIn_EKS_Cluster"
)

var annotations = map[string]interface{}{
	"aws_remote_target":      "remote-target",
	"aws_remote_operation":   "remote-operation",
	"aws_local_service":      "service-name",
	"aws_remote_service":     "service-name-remote",
	"HostedIn_K8s_Namespace": "default",
	"aws_local_operation":    "operation",
}

type AppSignalsTracesRunner struct {
	test_runner.BaseTestRunner
	testName    string
	clusterName string
}

func (t *AppSignalsTracesRunner) Validate() status.TestGroupResult {
	testResults := status.TestResult{
		Name:   t.testName,
		Status: status.FAILED,
	}
	timeNow := time.Now()
	annotations[EKSClusterAnnotation] = t.clusterName
	xrayFilter := awsservice.FilterExpression(annotations)
	traceIds, err := awsservice.GetTraceIDs(timeNow.Add(lookbackDuration), timeNow, xrayFilter)
	if err != nil {
		fmt.Printf("error getting trace ids: %v", err)
	} else {
		fmt.Printf("Trace IDs: %v\n", traceIds)
		if len(traceIds) > 0 {
			testResults.Status = status.SUCCESSFUL
		}
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: []status.TestResult{testResults},
	}
}

func (t *AppSignalsTracesRunner) GetTestName() string {
	return t.testName
}

func (t *AppSignalsTracesRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *AppSignalsTracesRunner) GetMeasuredMetrics() []string {
	return nil
}

func (e *AppSignalsTracesRunner) GetAgentConfigFileName() string {
	return ""
}

var _ test_runner.ITestRunner = (*AppSignalsTracesRunner)(nil)
