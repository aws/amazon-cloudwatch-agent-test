// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package test_runner

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

const (
	configOutputPath     = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentConfigDirectory = "agent_configs"
	MinimumAgentRuntime  = 3 * time.Minute
)

type ITestRunner interface {
	Validate() status.TestGroupResult
	GetTestName() string
	GetAgentConfigFileName() string
	GetAgentRunDuration() time.Duration
	GetMeasuredMetrics() []string
}

type TestRunner struct {
	TestRunner ITestRunner
}

type BaseTestRunner struct {
}

func (t *TestRunner) Run(s ITestSuite) {
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)
	testGroupResult, err := t.runAgent()
	if err == nil {
		testGroupResult = t.TestRunner.Validate()
	}
	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test group failed", testName)
	}
}

func (t *TestRunner) runAgent() (status.TestGroupResult, error) {
	testGroupResult := status.TestGroupResult{
		Name: t.TestRunner.GetTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.SUCCESSFUL,
			},
		},
	}

	agentConfigPath := filepath.Join(agentConfigDirectory, t.TestRunner.GetAgentConfigFileName())
	log.Printf("Starting agent using agent config file %s", agentConfigPath)
	test.CopyFile(agentConfigPath, configOutputPath)
	err := test.StartAgent(configOutputPath, false)

	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Agent could not start due to: %v", err.Error())
	}

	runningDuration := t.TestRunner.GetAgentRunDuration()
	time.Sleep(runningDuration)
	log.Printf("Agent has been running for : %s", runningDuration.String())
	test.StopAgent()

	err = test.DeleteFile(configOutputPath)
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to cleanup config file after agent run due to: %v", err.Error())
	}

	return testGroupResult, nil
}
