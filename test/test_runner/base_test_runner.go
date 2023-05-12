// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

const (
	configOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentConfigDirectory     = "agent_configs"
	extraConfigDirectory     = "extra_configs"
	MinimumAgentRuntime      = 1 * time.Minute
	HighResolutionStatPeriod = 30
)

type ITestRunner interface {
	Validate() status.TestGroupResult
	GetTestName() string
	GetAgentConfigFileName() string
	GetAgentRunDuration() time.Duration
	GetMeasuredMetrics() []string
	SetupBeforeAgentRun() error
	SetupAfterAgentRun() error
}

type TestRunner struct {
	TestRunner ITestRunner
}

type BaseTestRunner struct {
	DimensionFactory dimension.Factory
}

func (t *BaseTestRunner) SetupBeforeAgentRun() error {
	return nil
}

func (t *BaseTestRunner) SetupAfterAgentRun() error {
	return nil
}

func (t *BaseTestRunner) GetAgentRunDuration() time.Duration {
	return MinimumAgentRuntime
}

func (t *TestRunner) Run(s ITestSuite) {
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)
	/* 	This block is used to prevent userdata from running the agent
		post-launch since it should've been done on launch already in
		order to imitate customer behavoir
	*/
	testGroupResult := status.TestGroupResult{
		Name: t.TestRunner.GetTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Agent Started",
				Status: status.SUCCESSFUL,
			},
		},
	}
	err := error(nil)
	if testName == "Userdata" {
		testGroupResult = t.TestRunner.Validate()
	} else{
		testGroupResult, err = t.runAgent()
		if err == nil {
			testGroupResult = t.TestRunner.Validate()
		}
	}

	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test group failed due to %v", testName, err)
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

	err := t.TestRunner.SetupBeforeAgentRun()
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup before agent run due to: %w", err)
	}

	agentConfigPath := filepath.Join(agentConfigDirectory, t.TestRunner.GetAgentConfigFileName())
	log.Printf("Starting agent using agent config file %s", agentConfigPath)
	common.CopyFile(agentConfigPath, configOutputPath)
	err = common.StartAgent(configOutputPath, false)

	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Agent could not start due to: %w", err)
	}

	err = t.TestRunner.SetupAfterAgentRun()
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup after agent run due to: %w", err)
	}

	runningDuration := t.TestRunner.GetAgentRunDuration()
	time.Sleep(runningDuration)
	log.Printf("Agent has been running for : %s", runningDuration.String())
	common.StopAgent()

	err = common.DeleteFile(configOutputPath)
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to cleanup config file after agent run due to: %w", err)
	}

	return testGroupResult, nil
}
