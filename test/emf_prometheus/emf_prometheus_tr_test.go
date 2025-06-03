// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf_prometheus

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var tokenReplacementPrometheusConfig string

//go:embed resources/prometheus_metrics
var tokenReplacementPrometheusMetrics string

type TokenReplacementTestRunner struct {
	test_runner.BaseTestRunner
	namespace string
	jobName   string
}

const jobNamePrefix = "prometheus_job_"

func (t *TokenReplacementTestRunner) SetupBeforeAgentRun() error {
	instanceID := awsservice.GetInstanceId()
	t.namespace = fmt.Sprintf("%str_test_%s", namespacePrefix, instanceID)
	t.jobName = fmt.Sprintf("%str_test_%s", jobNamePrefix, instanceID)

	if err := setupPrometheus(tokenReplacementPrometheusConfig, tokenReplacementPrometheusMetrics, t.jobName); err != nil {
		return err
	}

	configPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	updatedContent := strings.ReplaceAll(string(content), "${NAMESPACE}", t.namespace)

	if err := os.WriteFile(configPath, []byte(updatedContent), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write updated config: %v", err)
	}

	common.CopyFile(configPath, common.ConfigOutputPath)

	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *TokenReplacementTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *TokenReplacementTestRunner) GetAgentRunDuration() time.Duration {
	return 1 * time.Minute
}

func (t *TokenReplacementTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{
		verifyLogGroupExists(t.jobName),
		VerifyMetricsInCloudWatch(t.namespace),
	}

	defer cleanup(t.jobName)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *TokenReplacementTestRunner) GetTestName() string {
	return "Prometheus EMF Token Replacement Test"
}

func (t *TokenReplacementTestRunner) GetAgentConfigFileName() string {
	return "emf_prometheus_tr_config.json"
}

func verifyLogGroupExists(logGroupName string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Log Group Existence",
		Status: status.FAILED,
	}

	maxAttempts := 3
	for i := 0; i < maxAttempts; i++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			log.Printf("Found log group %s", logGroupName)
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		log.Printf("Log group %s not found, attempt %d/%d, waiting...", logGroupName, i+1, maxAttempts)
		time.Sleep(30 * time.Second)
	}

	log.Printf("Failed to find log group %s after %d attempts", logGroupName, maxAttempts)
	return testResult
}
