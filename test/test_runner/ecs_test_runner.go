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
		return fmt.Errorf("Failed while reading config file: %s\n", configFilePath)
	}

	agentConfig := string(b)

	err = awsservice.PutStringParameter(e.CwagentConfigSsmParamName, agentConfig)
	if err != nil {
		return fmt.Errorf("Failed while reading config file : %s\n", err.Error())
	}
	fmt.Println("Put parameter successful.")

	err = awsservice.RestartDaemonService(e.EcsClusterArn, e.EcsServiceName)
	if err != nil {
		fmt.Print(err)
	}
	fmt.Println("CWAgent service is restarting. Sleeping..")

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

	// Store cleanup status
	var cleanupErr error
	defer func() {
		// Always attempt cleanup after test completion
		log.Printf("Performing ECS log cleanup for test: %s", name)
		cleanupErr = t.performCleanup()
		if cleanupErr != nil {
			log.Printf("ECS cleanup completed with warnings: %v", cleanupErr)
		}
	}()

	//runs agent restart with given config only when it's available
	agentConfigFileName := t.Runner.GetAgentConfigFileName()
	if len(agentConfigFileName) != 0 {
		err := t.RunStrategy.RunAgentStrategy(e, t.Runner.GetAgentConfigFileName())
		if err != nil {
			log.Printf("Failed to run agent with config for the given test err: %v\n", err)
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
	} else {
		log.Printf("CloudWatch Agent started.")
	}

	testGroupResult := t.Runner.Validate()

	// Add cleanup status if there were issues
	if cleanupErr != nil {
		testGroupResult.TestResults = append(testGroupResult.TestResults, status.TestResult{
			Name:   "Log Cleanup",
			Status: status.SUCCESSFUL, // Don't fail tests due to cleanup issues
			Reason: fmt.Errorf("cleanup completed with warnings: %w", cleanupErr),
		})
	}

	s.AddToSuiteResult(testGroupResult)
	if testGroupResult.GetStatus() != status.SUCCESSFUL {
		log.Printf("%s test group failed", name)
	}
}

// performCleanup handles cleanup of Container Insights and EMF logs specific to ECS
func (t *ECSTestRunner) performCleanup() error {
	// Check if cleanup is disabled
	if skipCleanup := os.Getenv("CWAGENT_SKIP_LOG_CLEANUP"); skipCleanup == "true" {
		log.Printf("ECS log cleanup skipped due to CWAGENT_SKIP_LOG_CLEANUP environment variable")
		return nil
	}

	// ECS-specific cleanup patterns
	ecsCleanupConfig := awsservice.LogGroupCleanupConfig{
		IncludePatterns: []string{
			"/aws/ecs/containerinsights/.*/performance",
			"/aws/ecs/containerinsights/.*/application", 
			"/ecs/.*",
			".*EMFECSNameSpace.*",
			"cwagent-ecs-.*",
		},
		ExcludePatterns: []string{
			".*production.*",
			".*prod.*",
		},
		DryRun: os.Getenv("CWAGENT_FORCE_LOG_CLEANUP") != "true",
	}

	// Add age constraint for safety
	maxAge := 2 * time.Hour // Only clean logs older than 2 hours for ECS
	ecsCleanupConfig.MaxAge = &maxAge

	log.Printf("Starting ECS-specific log cleanup (dry run: %v)", ecsCleanupConfig.DryRun)
	result, err := awsservice.CleanupLogGroupsByPattern(ecsCleanupConfig)
	if err != nil {
		return fmt.Errorf("ECS log cleanup failed: %w", err)
	}

	log.Printf("ECS log cleanup completed. Deleted: %d, Skipped: %d, Errors: %d", 
		len(result.DeletedLogGroups), len(result.SkippedLogGroups), len(result.Errors))

	if len(result.Errors) > 0 {
		return fmt.Errorf("ECS cleanup completed with %d errors", len(result.Errors))
	}

	return nil
}
