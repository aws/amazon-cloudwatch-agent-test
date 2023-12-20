// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package app_signals

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	lookbackDuration     = time.Duration(-5) * time.Minute
	EKSClusterAnnotation = "HostedIn_EKS_Cluster"
	EC2Annotation        = "HostedIn_Environment"
)

var annotations = map[string]interface{}{
	"aws_remote_target":    "remote-target",
	"aws_remote_operation": "remote-operation",
	"aws_local_service":    "service-name",
	"aws_remote_service":   "service-name-remote",
	"aws_local_operation":  "replaced-operation",
}

type AppSignalsTracesRunner struct {
	test_runner.BaseTestRunner
	testName    string
	hostedIn    string
	computeType computetype.ComputeType
}

func (t *AppSignalsTracesRunner) Validate() status.TestGroupResult {
	testResults := status.TestResult{
		Name:   t.testName,
		Status: status.FAILED,
	}
	timeNow := time.Now()

	// "Generic" means EC2
	if t.hostedIn == "Generic" {
		annotations[EC2Annotation] = t.hostedIn
	} else {
		annotations[EKSClusterAnnotation] = t.hostedIn
		annotations["HostedIn_K8s_Namespace"] = "default"
	}

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
	return "config.json"
}

func (e *AppSignalsTracesRunner) SetupAfterAgentRun() error {
	// sends metrics data only for EC2
	if e.computeType == computetype.EC2 {
		cmd := `while true; chmod +x ./resources/traceid_generator.go; export START_TIME=$(date +%s%N); export TRACE_ID=$(go run ./resources/traceid_generator.go); do 
			cat ./resources/traces/traces.json | sed -e "s/START_TIME/$START_TIME/" | sed -e "s/TRACE_ID/$TRACE_ID/" > traces.json; 
			curl -H 'Content-Type: application/json' -d @traces.json -i http://127.0.0.1:4316/v1/traces --verbose; 
			sleep 5; done`
		return common.RunAsyncCommand(cmd)
	}

	return nil
}

var _ test_runner.ITestRunner = (*AppSignalsTracesRunner)(nil)
