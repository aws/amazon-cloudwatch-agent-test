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
	"strconv"
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
	CleanupAfterTest() error // New method for cleanup
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

// CleanupAfterTest provides default cleanup behavior for EMF and Container Insights logs
func (t *BaseTestRunner) CleanupAfterTest() error {
	// Check if cleanup is disabled via environment variable
	if skipCleanup, _ := strconv.ParseBool(os.Getenv("CWAGENT_SKIP_LOG_CLEANUP")); skipCleanup {
		log.Printf("Log cleanup skipped due to CWAGENT_SKIP_LOG_CLEANUP environment variable")
		return nil
	}

	// Check if we should do a dry run (default behavior for safety)
	dryRun := true
	if forceCleanup, _ := strconv.ParseBool(os.Getenv("CWAGENT_FORCE_LOG_CLEANUP")); forceCleanup {
		dryRun = false
		log.Printf("Performing actual log cleanup due to CWAGENT_FORCE_LOG_CLEANUP environment variable")
	}

	// Perform cleanup
	log.Printf("Starting log cleanup (dry run: %v)", dryRun)
	err := awsservice.CleanupTestLogGroups(dryRun)
	if err != nil {
		log.Printf("Warning: Log cleanup failed: %v", err)
		// Don't fail the test due to cleanup issues
		return nil
	}

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

// Run executes the test and includes cleanup
func (t *TestRunner) Run() status.TestGroupResult {
	testName := t.TestRunner.GetTestName()
	log.Printf("Running %v", testName)
	
	// Store cleanup error separately to avoid masking test results
	var cleanupErr error
	defer func() {
		// Always attempt cleanup, even if test failed
		log.Printf("Performing cleanup for test: %s", testName)
		cleanupErr = t.TestRunner.CleanupAfterTest()
		if cleanupErr != nil {
			log.Printf("Cleanup completed with warnings: %v", cleanupErr)
		}
	}()
	
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
	
	result := t.TestRunner.Validate()
	
	// Add cleanup status to test results if there were any issues
	if cleanupErr != nil {
		result.TestResults = append(result.TestResults, status.TestResult{
			Name:   "Log Cleanup",
			Status: status.SUCCESSFUL, // We don't fail tests due to cleanup issues
			Reason: fmt.Errorf("cleanup completed with warnings: %w", cleanupErr),
		})
	}
	
	return result
}

func (t *TestRunner) RunAgent() error {
	agentConfig := AgentConfig{
		ConfigFileName:   t.TestRunner.GetAgentConfigFileName(),
		SSMParameterName: t.TestRunner.SSMParameterName(),
		UseSSM:           t.TestRunner.UseSSM(),
	}
	t.TestRunner.SetAgentConfig(agentConfig)
	err := t.TestRunner.SetupBeforeAgentRun()
	if err != nil {
		return fmt.Errorf("Failed to complete setup before agent run due to: %w", err)
	}

	if t.TestRunner.UseSSM() {
		err = common.StartAgent(t.TestRunner.SSMParameterName(), false, true)
	} else {
		err = common.StartAgent(configOutputPath, false, false)
	}

	if err != nil {
		return fmt.Errorf("Agent could not start due to: %w", err)
	}

	err = t.TestRunner.SetupAfterAgentRun()
	if err != nil {
		return fmt.Errorf("Failed to complete setup after agent run due to: %w", err)
	}

	runningDuration := t.TestRunner.GetAgentRunDuration()
	time.Sleep(runningDuration)
	log.Printf("Agent has been running for : %s", runningDuration.String())
	common.StopAgent()

	err = common.DeleteFile(configOutputPath)
	if err != nil {
		return fmt.Errorf("Failed to cleanup config file after agent run due to: %w", err)
	}

	return nil
}
