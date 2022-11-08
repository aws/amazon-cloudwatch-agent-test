// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
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

type ECSTestRunner struct {
	testRunner IECSTestRunner
}

func (t *ECSTestRunner) Run(s *MetricBenchmarkTestSuite, cwagentConfigSsmParamName *string, clusterArn *string, serviceName *string) {
	testName := t.testRunner.getTestName()
	log.Printf("Running %v", testName)
	testGroupResult, err := t.runAgent(cwagentConfigSsmParamName, clusterArn, serviceName)
	if err == nil {
		testGroupResult = t.testRunner.validate()
	}
	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test group failed", testName)
	}
}

func (t *ECSTestRunner) runAgent(cwagentConfigSsmParamName *string, clusterArn *string, serviceName *string) (status.TestGroupResult, error) {
	// 1) First, make a always-failing test so I can keep rerunning the test in my fork. make ca tests always succeed temporarily to save iterating time
	// get flag to benchmark test, and benchmark test creates the right base test class with the right runAgent() based on ecs vs ec2 -> make agentRunner injectable. -> test by logging
	// this runAgent should
	// 1) put new config file to ssm through ssm sdk -> test on fork and verify in my account
	// 2) start agent with the new ssm file: probably means restarting task/container/daemon service. ecs sdk.  -> test if new metrics with cpu gets emitted
	// 3) use the same validation logic

	log.Printf("ECS runAgent Base Test")
	log.Printf("ECS CWAgent Config SSM Parameter Name is %s", *cwagentConfigSsmParamName)
	b, err := os.ReadFile("./agent_configs/cpu_config.json")
	if err != nil {
		fmt.Print(err)
		testGroupResult := status.TestGroupResult{
			Name: t.testRunner.getTestName(),
			TestResults: []status.TestResult{
				{
					Name:   "Starting Agent",
					Status: status.FAILED,
				},
			},
		}
		return testGroupResult, fmt.Errorf("Failed while reading config file")
	}

	agentConfig := string(b)
	ssmParamType := "String"

	err = test.PutParameter(cwagentConfigSsmParamName, &agentConfig, &ssmParamType)
	if err != nil {
		fmt.Print(err)
	}
	log.Printf("Put parameter happened")

	taskArns, err := test.ListTasks(clusterArn, serviceName)
	if err != nil {
		fmt.Print(err)
	}
	if len(taskArns) <= 0 {
		fmt.Errorf("Task arns aren't expected to be 0. It should be 1")
	}
	cwagentTaskArn := taskArns[0]
	log.Printf("CWAgent task arn found: %s", *cwagentTaskArn)
	log.Printf("ListTasks happened in order to stop and start the task again")

	err = test.StopTask(clusterArn, cwagentTaskArn)
	if err != nil {
		fmt.Print(err)
	}
	log.Printf("CWAgent task is stopped")

	//TODO for each failure cases, fail early (by returning pre-made failure state)

	testGroupResult := status.TestGroupResult{
		Name: t.testRunner.getTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.FAILED,
			},
		},
	}

	return testGroupResult, fmt.Errorf("Default failure for development test")
	/*
		agentConfigPath := filepath.Join(agentConfigDirectory, t.testRunner.getAgentConfigFileName())
		log.Printf("Starting agent using agent config file %s", agentConfigPath)
		test.CopyFile(agentConfigPath, configOutputPath)
		err := test.StartAgent(configOutputPath, false)

		if err != nil {
			testGroupResult.TestResults[0].Status = status.FAILED
			return testGroupResult, fmt.Errorf("Agent could not start due to: %v", err.Error())
		}

		runningDuration := t.testRunner.getAgentRunDuration()
		time.Sleep(runningDuration)
		log.Printf("Agent has been running for : %s", runningDuration.String())
		test.StopAgent()

		err = test.DeleteFile(configOutputPath)
		if err != nil {
			testGroupResult.TestResults[0].Status = status.FAILED
			return testGroupResult, fmt.Errorf("Failed to cleanup config file after agent run due to: %v", err.Error())
		}
	*/
	return testGroupResult, nil
}
