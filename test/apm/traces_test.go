// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package apm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/xray"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	lookbackDuration     = time.Duration(-80) * time.Minute
	EKSClusterAnnotation = "HostedIn_EKS_Cluster"
)

var annotations = map[string]string{
	"aws_remote_target":      "remote-target",
	"aws_remote_operation":   "remote-operation",
	"aws_local_service":      "service-name",
	"aws_remote_service":     "service-name-remote",
	"HostedIn_K8s_Namespace": "default",
	"aws_local_operation":    "operation",
}

type APMTracesRunner struct {
	test_runner.BaseTestRunner
	testName    string
	clusterName string
}

func (t *APMTracesRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{{
		Name:   t.testName,
		Status: status.FAILED,
	}}
	timeNow := time.Now()
	annotations[EKSClusterAnnotation] = t.clusterName
	xrayFilter := FilterExpression(annotations)
	traceIds, err := GetTraceIDs(timeNow.Add(lookbackDuration), timeNow, xrayFilter)
	if err != nil {
		fmt.Printf("error getting trace ids: %v", err)
	} else {
		fmt.Printf("Trace IDs: %v\n", traceIds)
		if len(traceIds) > 0 {
			testResults[0].Status = status.SUCCESSFUL
		}
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *APMTracesRunner) GetTestName() string {
	return t.testName
}

func (t *APMTracesRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *APMTracesRunner) GetMeasuredMetrics() []string {
	return nil
}

func (e *APMTracesRunner) GetAgentConfigFileName() string {
	return ""
}

func GetTraceIDs(startTime time.Time, endTime time.Time, filter string) ([]string, error) {
	var traceIDs []string
	input := &xray.GetTraceSummariesInput{StartTime: aws.Time(startTime), EndTime: aws.Time(endTime), FilterExpression: aws.String(filter)}
	output, err := awsservice.XrayClient.GetTraceSummaries(context.Background(), input)
	if err != nil {
		return nil, err
	}
	for _, summary := range output.TraceSummaries {
		traceIDs = append(traceIDs, *summary.Id)
	}
	return traceIDs, nil
}

func FilterExpression(annotations map[string]string) string {
	var expression string
	for key, value := range annotations {
		result, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if len(expression) != 0 {
			expression += " AND "
		}
		expression += fmt.Sprintf("annotation.%s = %s", key, result)
	}
	return expression
}

var _ test_runner.ITestRunner = (*APMTracesRunner)(nil)
