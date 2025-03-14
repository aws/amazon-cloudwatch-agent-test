// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package test_runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
	cleanup          *CleanupHandler
	ctx              context.Context
	cancelFunc       context.CancelFunc
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

// AddCleanup registers a cleanup function to be executed on test completion or cancellation
func (t *BaseTestRunner) AddCleanup(fn func() error) {
	if t.cleanup != nil {
		t.cleanup.AddCleanup(fn)
	}
}

// RunCleanup executes all registered cleanup functions
func (t *BaseTestRunner) RunCleanup() {
	if t.cleanup != nil {
		t.cleanup.RunCleanup()
	}
}

func (t *TestRunner) Run() status.TestGroupResult {
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			log.Printf("Received termination signal for test: %s", testName)
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	// Initialize cleanup handler
	if runner, ok := t.TestRunner.(*BaseTestRunner); ok {
		runner.cleanup = &CleanupHandler{}
		runner.ctx = ctx
		runner.cancelFunc = cancel
	}

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
	t.TestRunner.SetAgentConfig(agentConfig)
	
	// Register cleanup for config file
	if runner, ok := t.TestRunner.(*BaseTestRunner); ok && runner.cleanup != nil {
		runner.cleanup.AddCleanup(func() error {
			log.Printf("Cleaning up config file: %s", configOutputPath)
			return common.DeleteFile(configOutputPath)
		})
	}
	
	err := t.TestRunner.SetupBeforeAgentRun()
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup before agent run due to: %w", err)
	}

	if t.TestRunner.UseSSM() {
		err = common.StartAgent(t.TestRunner.SSMParameterName(), false, true)
	} else {
		err = common.StartAgent(configOutputPath, false, false)
	}

	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Agent could not start due to: %w", err)
	}

	// Register agent stop in cleanup
	if runner, ok := t.TestRunner.(*BaseTestRunner); ok && runner.cleanup != nil {
		runner.cleanup.AddCleanup(func() error {
			log.Printf("Stopping agent as part of cleanup")
			common.StopAgent()
			return nil
		})
	}

	err = t.TestRunner.SetupAfterAgentRun()
	if err != nil {
		testGroupResult.TestResults[0].Status = status.FAILED
		return testGroupResult, fmt.Errorf("Failed to complete setup after agent run due to: %w", err)
	}

	runningDuration := t.TestRunner.GetAgentRunDuration()
	time.Sleep(runningDuration)
	log.Printf("Agent has been running for : %s", runningDuration.String())
	
	// Run cleanup in normal flow
	if runner, ok := t.TestRunner.(*BaseTestRunner); ok && runner.cleanup != nil {
		runner.cleanup.RunCleanup()
	} else {
		// Fallback to original behavior if cleanup handler not available
		common.StopAgent()
		err = common.DeleteFile(configOutputPath)
		if err != nil {
			testGroupResult.TestResults[0].Status = status.FAILED
			return testGroupResult, fmt.Errorf("Failed to cleanup config file after agent run due to: %w", err)
		}
	}

	return testGroupResult, nil
}
