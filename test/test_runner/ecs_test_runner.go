// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"log"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
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

	time.Sleep(5 * time.Minute)

	return nil
}

type ECSTestRunner struct {
	Runner      ITestRunner
	RunStrategy IAgentRunStrategy
	Env         environment.MetaData
}

func (t *ECSTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *ECSTestRunner) SetupAfterAgentRun() error {
	return nil
}

func (t *ECSTestRunner) Run(s ITestSuite, e *environment.MetaData) {
	name := t.Runner.GetTestName()
	log.Printf("Running %s", name)

	testGroupResult := t.Runner.Validate()

	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}
