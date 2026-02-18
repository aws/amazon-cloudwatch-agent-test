// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/profiling"
)

const (
	agentConfigDirectory = "agent_configs"
)

type ITestRunner interface {
	Validate() status.TestGroupResult
	GetTestName() string
	GetAgentConfigFileName() string
	GetAgentRunDuration() time.Duration
	GetMeasuredMetrics() []string
	SetupBeforeAgentRun() error
	SetupAfterAgentRun() error
	UseSSM() bool
	SSMParameterName() string
	SetUpConfig() error
	SetAgentConfig(config AgentConfig)
	RegisterCleanup(f func() error)
	Cleanup()
}

type TestRunner struct {
	TestRunner ITestRunner
}

type BaseTestRunner struct {
	DimensionFactory dimension.Factory
	AgentConfig      AgentConfig
	CleanupFns       []func() error
}

type AgentConfig struct {
	ConfigFileName   string
	SSMParameterName string
	UseSSM           bool
}

func (t *BaseTestRunner) SetupBeforeAgentRun() error {
	return t.SetUpConfig()
}

func (t *BaseTestRunner) SetUpConfig() error {
	agentConfigPath := filepath.Join(agentConfigDirectory, t.AgentConfig.ConfigFileName)
	log.Printf("Starting agent using agent config file %s", agentConfigPath)
	common.CopyFile(agentConfigPath, common.ConfigOutputPath)
	if t.AgentConfig.UseSSM {
		log.Printf("Starting agent from ssm parameter %s", agentConfigPath)
		agentConfigByteArray, err := os.ReadFile(agentConfigPath)
		if err != nil {
			return errors.New("failed while reading config file")
		}
		agentConfig := string(agentConfigByteArray)
		if agentConfig != awsservice.GetStringParameter(t.AgentConfig.SSMParameterName) {
			log.Printf("ssm agent config %s canged upload new config", t.AgentConfig.SSMParameterName)
			err = awsservice.PutStringParameter(t.AgentConfig.SSMParameterName, agentConfig)
			if err != nil {
				return fmt.Errorf("failed to upload ssm parameter err %v", err)
			}
		}
	}
	return nil
}

func (t *BaseTestRunner) SetupAfterAgentRun() error {
	return nil
}

func (t *BaseTestRunner) GetAgentRunDuration() time.Duration {
	return 30 * time.Second
}

func (t *BaseTestRunner) UseSSM() bool {
	return false
}

func (t *BaseTestRunner) SSMParameterName() string {
	return ""
}

func (t *BaseTestRunner) SetAgentConfig(agentConfig AgentConfig) {
	t.AgentConfig = agentConfig
}

func (t *BaseTestRunner) RegisterCleanup(f func() error) {
	t.CleanupFns = append(t.CleanupFns, f)
}

func (t *BaseTestRunner) Cleanup() {
	for _, cleanupFn := range t.CleanupFns {
		if err := cleanupFn(); err != nil {
			log.Printf("Failed to cleanup test runner: %v", err)
		}
	}
	t.CleanupFns = nil
}

func (t *TestRunner) Run() status.TestGroupResult {
	defer t.TestRunner.Cleanup()
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)

	prof := profiling.Global()
	prof.StartTest(testName)
	defer prof.EndTest(testName)

	err := t.RunAgent()
	if err != nil {
		log.Printf("%v test group failed while running agent: %v", testName, err)
		return status.TestGroupResult{
			Name: t.TestRunner.GetTestName(),
			TestResults: []status.TestResult{
				{
					Name:   "Starting Agent",
					Status: status.FAILED,
					Reason: err,
				},
			},
		}
	}

	timer := prof.StartSpan(testName, "Validate", profiling.CategoryValidation)
	result := t.TestRunner.Validate()
	timer.Stop()
	return result
}

func (t *TestRunner) RunAgent() error {
	testName := t.TestRunner.GetTestName()
	prof := profiling.Global()

	agentConfig := AgentConfig{
		ConfigFileName:   t.TestRunner.GetAgentConfigFileName(),
		SSMParameterName: t.TestRunner.SSMParameterName(),
		UseSSM:           t.TestRunner.UseSSM(),
	}
	t.TestRunner.SetAgentConfig(agentConfig)

	timer := prof.StartSpan(testName, "SetupBeforeAgentRun", profiling.CategorySetup)
	err := t.TestRunner.SetupBeforeAgentRun()
	timer.Stop()
	if err != nil {
		return fmt.Errorf("Failed to complete setup before agent run due to: %w", err)
	}

	timer = prof.StartSpan(testName, "StartAgent", profiling.CategorySetup)
	if t.TestRunner.UseSSM() {
		err = common.StartAgent(t.TestRunner.SSMParameterName(), false, true)
	} else {
		err = common.StartAgent(common.ConfigOutputPath, false, false)
	}
	timer.Stop()

	if err != nil {
		return fmt.Errorf("Agent could not start due to: %w", err)
	}

	timer = prof.StartSpan(testName, "SetupAfterAgentRun", profiling.CategorySetup)
	err = t.TestRunner.SetupAfterAgentRun()
	timer.Stop()
	if err != nil {
		return fmt.Errorf("Failed to complete setup after agent run due to: %w", err)
	}

	runningDuration := t.TestRunner.GetAgentRunDuration()
	timer = prof.StartSpan(testName, fmt.Sprintf("AgentRunSleep(%s)", runningDuration), profiling.CategoryAgentWait)
	time.Sleep(runningDuration)
	timer.Stop()
	log.Printf("Agent has been running for : %s", runningDuration.String())
	common.StopAgent()

	err = common.DeleteFile(common.ConfigOutputPath)
	if err != nil {
		return fmt.Errorf("Failed to cleanup config file after agent run due to: %w", err)
	}

	return nil
}
