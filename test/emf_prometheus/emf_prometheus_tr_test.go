package emf_prometheus

import (
	_ "embed"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//go:embed resources/prometheus.yaml
var tokenReplacementPrometheusConfig string

//go:embed resources/prometheus_metrics
var tokenReplacementPrometheusMetrics string

type TokenReplacementTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *TokenReplacementTestRunner) GetMeasuredMetrics() []string {
	return nil
}
func (t *TokenReplacementTestRunner) Validate() status.TestGroupResult {
	randomSuffix := generateRandomSuffix()
	jobName := fmt.Sprintf("%s_tr_test_%s", "prometheus", randomSuffix)
	namespace := fmt.Sprintf("%s_tr_test_%s", namespacePrefix, randomSuffix)

	if err := setupPrometheus(tokenReplacementPrometheusConfig, tokenReplacementPrometheusMetrics, jobName); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:   "Setup",
				Status: status.FAILED,
			}},
		}
	}

	if err := startAgent(filepath.Join("agent_configs", "emf_prometheus_config.json"), namespace, ""); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:   "Agent Start",
				Status: status.FAILED,
			}},
		}
	}

	defer cleanup(jobName)

	testResults := []status.TestResult{
		verifyLogGroupExists(jobName),
		verifyMetricsInCloudWatch(namespace),
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *TokenReplacementTestRunner) GetTestName() string {
	return "Prometheus EMF Token Replacement Test"
}

func (t *TokenReplacementTestRunner) GetAgentConfigFileName() string {
	return filepath.Join("agent_configs", "emf_prometheus_token_replacement_config.json")
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
