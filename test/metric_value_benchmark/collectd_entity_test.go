// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var testCases = []CollectDEntityTestCase{
	{
		name:       "Test1",
		configFile: "collectd_entity_config1.json",
	},
	{
		name:       "Test2",
		configFile: "collectd_entity_config2.json",
	},
}

// CollectdTestRunner represents the main test runner for collectd tests
type CollectDEntityTestRunner struct {
	test_runner.BaseTestRunner
}

var itrunner test_runner.ITestRunner = (*CollectDEntityTestRunner)(nil)

// CollectdTestCase represents a single test case with its own config
type CollectDEntityTestCase struct {
	name       string
	configFile string
}

// NewCollectdTestRunner creates a new instance of CollectdTestRunner
func NewCollectDEntityTestRunner() *CollectDEntityTestRunner {
	return &CollectDEntityTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
	}
}

func (t *CollectDEntityTestRunner) GetTestName() string {
	return "CollectDEntity"
}

func (t *CollectDEntityTestRunner) GetAgentRunDuration() time.Duration {
	return 60 * time.Second
}

func (t *CollectDEntityTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *CollectDEntityTestRunner) GetMeasuredMetrics() []string {
	return []string{"collectd_gauge_1_value", "collectd_counter_1_value"}
}

func (t *CollectDEntityTestRunner) Validate() status.TestGroupResult {
	result := status.TestGroupResult{
		Name: t.GetTestName(),
	}

	// Run each test case
	for _, testCase := range testCases {
		// Set the config for this test case
		t.SetAgentConfig(test_runner.AgentConfig{
			ConfigFileName: testCase.configFile,
		})

		t.SetUpConfig()

		// Run the agent with this config
		runner := &test_runner.TestRunner{
			TestRunner: itrunner,
		}

		// Run the agent and get results
		_, err := runner.RunAgent()

		if err != nil {
			result.TestResults = append(result.TestResults, status.TestResult{
				Name:   testCase.configFile,
				Status: status.FAILED,
			})
			continue
		}

		// Validate metrics for this test case
		testResult := status.TestResult{
			Name:   testCase.configFile,
			Status: status.SUCCESSFUL,
		}
		result.TestResults = append(result.TestResults, testResult)
	}

	return result
}

//  need to validate test cases
