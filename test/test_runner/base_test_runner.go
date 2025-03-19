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
)

const (
	configOutputPath     = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentConfigDirectory = "agent_configs"
	extraConfigDirectory = "extra_configs"
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
}

type TestRunner struct {
	TestRunner ITestRunner
}

type BaseTestRunner struct {
	DimensionFactory dimension.Factory
	AgentConfig      AgentConfig
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
	log.Println("hi123")

	agentConfigPath := filepath.Join(agentConfigDirectory, t.AgentConfig.ConfigFileName)
	log.Printf("Starting agent using agent config file %s", agentConfigPath)
	common.CopyFile(agentConfigPath, configOutputPath)
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

func (t *TestRunner) Run() status.TestGroupResult {
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)
	testGroupResult, err := t.RunAgent()
	if err == nil {
		testGroupResult = t.TestRunner.Validate()
	}
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%v test group failed due to %v", testName, err)
	}

	return testGroupResult
}

func (t *TestRunner) RunAgent() (status.TestGroupResult, error) {
	log.Println("hi0")

	testGroupResult := status.TestGroupResult{
		Name: t.TestRunner.GetTestName(),
		TestResults: []status.TestResult{
			{
				Name:   "Starting Agent",
				Status: status.SUCCESSFUL,
			},
		},
	}

	agentConfig := AgentConfig{
		ConfigFileName:   t.TestRunner.GetAgentConfigFileName(),
		SSMParameterName: t.TestRunner.SSMParameterName(),
		UseSSM:           t.TestRunner.UseSSM(),
	}
	log.Println("hi1")
	t.TestRunner.SetAgentConfig(agentConfig)
	log.Println("hi12")

	err := t.TestRunner.SetupBeforeAgentRun()
	log.Println("hi123")

	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup before agent run due to: %w", err)
	}
	log.Println("hi2")

	if t.TestRunner.UseSSM() {

		err = common.StartAgent(t.TestRunner.SSMParameterName(), false, true)
	} else {
		err = common.StartAgent(configOutputPath, false, false)
	}
	log.Println("hi3")

	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Agent could not start due to: %w", err)
	}
	log.Println("hi4")

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
