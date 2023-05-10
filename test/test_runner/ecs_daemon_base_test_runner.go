// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type IAgentRunStrategy interface {
	RunAgent(e *environment.MetaData, configFilePath string) error
}

type ECSAgentRunStrategy struct {
}

type IECSTestRunner interface {
	Validate(e *environment.MetaData) status.TestGroupResult
	GetTestName() string
	GetAgentConfigFileName() string
	GetAgentRunDuration() time.Duration
	GetMeasuredMetrics() []string
	SetupAfterAgentRun() error
}

type ECSTestRunner struct {
	TestRunner       IECSTestRunner
	AgentRunStrategy IAgentRunStrategy
	Env              environment.MetaData
}

type ECSBaseTestRunner struct {
	DimensionFactory dimension.Factory
}

func (t *ECSBaseTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *ECSBaseTestRunner) SetupAfterAgentRun() error {
	return nil
}

func (t *ECSTestRunner) Run(s ITestSuite, e *environment.MetaData) {
	testName := t.TestRunner.GetTestName()
	fmt.Printf("Running %s", testName)
	testGroupResult, err := t.runAgent(e)
	if err == nil {
		testGroupResult = t.TestRunner.Validate(e)
	}

	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		fmt.Printf("%s test group failed", testName)
	}
}

func (t *ECSTestRunner) runAgent(e *environment.MetaData) (status.TestGroupResult, error) {
	testGroupResult := status.TestGroupResult{
		Name: t.TestRunner.GetTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.FAILED,
			},
		},
	}

	var err error
	//runs agent restart with given config only when it's available
	agentConfigFileName := t.TestRunner.GetAgentConfigFileName()
	if len(agentConfigFileName) != 0 {
		err = t.AgentRunStrategy.RunAgent(e, t.TestRunner.GetAgentConfigFileName())
		if err != nil {
			fmt.Print(err)
			return testGroupResult, fmt.Errorf("Failed to run agent with config for the given test")
		}
	}

	err = t.TestRunner.SetupAfterAgentRun()
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup after agent run due to: %w", err)
	}

	testGroupResult.TestResults[0].Status = status.SUCCESSFUL
	return testGroupResult, nil
}

func (r *ECSAgentRunStrategy) RunAgent(e *environment.MetaData, configFilePath string) error {
	b, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("Failed while reading config file")
	}

	agentConfig := string(b)

	err = awsservice.PutStringParameter(e.CwagentConfigSsmParamName, agentConfig)
	if err != nil {
		return fmt.Errorf("Failed while reading config file : %s", err.Error())
	}
	fmt.Print("Put parameter successful")

	err = awsservice.RestartDaemonService(e.EcsClusterArn, e.EcsServiceName)
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print("CWAgent service is restarted")

	time.Sleep(5 * time.Minute)

	return nil
}
