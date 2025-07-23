// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type IAgentRunStrategy interface {
	RunAgentStrategy(e *environment.MetaData, configFilePath string) error
}

type ECSAgentRunStrategy struct {
}

func (r *ECSAgentRunStrategy) RunAgentStrategy(e *environment.MetaData, configFilePath string) error {
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

	time.Sleep(1 * time.Minute)

	return nil
}

type ECSTestRunner struct {
	Runner      ITestRunner
	RunStrategy IAgentRunStrategy
	Env         environment.MetaData
}

func (t *ECSTestRunner) Run(s ITestSuite, e *environment.MetaData) {
	name := t.Runner.GetTestName()
	log.Printf("Running %s", name)

	//runs agent restart with given config only when it's available
	agentConfigFileName := t.Runner.GetAgentConfigFileName()
	if len(agentConfigFileName) != 0 {
		err := t.RunStrategy.RunAgentStrategy(e, t.Runner.GetAgentConfigFileName())
		if err != nil {
			log.Printf("Failed to run agent with config for the given test err:%v", err)
			s.AddToSuiteResult(status.TestGroupResult{
				Name: t.Runner.GetTestName(),
				TestResults: []status.TestResult{
					{
						Name:   "Starting Agent",
						Status: status.FAILED,
					},
				},
			})
			return
		}
	}

	testGroupResult := t.Runner.Validate()

	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}
