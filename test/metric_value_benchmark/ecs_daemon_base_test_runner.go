// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"log"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

type IECSTestRunner interface {
	validate() status.TestGroupResult
	getTestName() string
	getAgentConfigFileName() string
	getAgentRunDuration() time.Duration
	getMeasuredMetrics() []string
}

type IAgentRunStrategy interface {
	runAgent(e *environment.MetaData, configFilePath string) error
}

type ECSAgentRunStrategy struct {
}

func (r *ECSAgentRunStrategy) runAgent(e *environment.MetaData, configFilePath string) error {
	log.Printf("ECS CWAgent Config SSM Parameter Name is %s", e.CwagentConfigSsmParamName)
	b, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("Failed while reading config file")
	}

	agentConfig := string(b)
	ssmParamType := "String"

	err = test.PutParameter(&(e.CwagentConfigSsmParamName), &agentConfig, &ssmParamType)
	if err != nil {
		return fmt.Errorf("Failed while reading config file : %s", err.Error())
	}
	log.Printf("Put parameter successful")

	err = test.RestartDaemonService(&(e.EcsClusterArn), &(e.EcsServiceName))
	if err != nil {
		fmt.Print(err)
	}
	log.Printf("CWAgent service is restarted")

	time.Sleep(10 * time.Minute)

	return nil
}

type ECSTestRunner struct {
	testRunner       IECSTestRunner
	agentRunStrategy IAgentRunStrategy
}

func (t *ECSTestRunner) Run(s *MetricBenchmarkTestSuite, e *environment.MetaData) {
	testName := t.testRunner.getTestName()
	log.Printf("Running %v", testName)
	testGroupResult, err := t.runAgent(e)
	if err == nil {
		testGroupResult = t.testRunner.validate()
	}
	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test group failed", testName)
	}
}

func (t *ECSTestRunner) runAgent(e *environment.MetaData) (status.TestGroupResult, error) {
	testGroupResult := status.TestGroupResult{
		Name: t.testRunner.getTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.FAILED,
			},
		},
	}

	log.Printf("ECS CWAgent Config SSM Parameter Name is %s", e.CwagentConfigSsmParamName)
	err := t.agentRunStrategy.runAgent(e, "./agent_configs/container_insights.json")

	if err != nil {
		fmt.Print(err)
		return testGroupResult, fmt.Errorf("Failed to run agent with config for the given test")
	}

	return testGroupResult, fmt.Errorf("Default failure for development test")
}
