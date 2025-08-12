// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf_prometheus

import (
	_ "embed"
	"encoding/json"
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

//go:embed resources/prometheus_relabel.yaml
var relabelPrometheusConfig string

//go:embed resources/prometheus_metrics
var relabelPrometheusMetrics string

type RelabelTestRunner struct {
	test_runner.BaseTestRunner
	namespace    string
	logGroupName string
}

func (t *RelabelTestRunner) SetupBeforeAgentRun() error {
	instanceID := awsservice.GetInstanceId()
	t.namespace = fmt.Sprintf("%srelabel_test_%s", namespacePrefix, instanceID)
	t.logGroupName = fmt.Sprintf("%srelabel_test_%s", logGroupPrefix, instanceID)
	log.Println("This is the namespace and the logGroupName", t.namespace, t.logGroupName)
	if err := setupPrometheus(relabelPrometheusConfig, relabelPrometheusMetrics, ""); err != nil {
		return err
	}

	configPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	updatedContent := strings.ReplaceAll(string(content), "${NAMESPACE}", t.namespace)
	updatedContent = strings.ReplaceAll(updatedContent, "${LOG_GROUP_NAME}", t.logGroupName)

	log.Println(updatedContent)
	if err := os.WriteFile(configPath, []byte(updatedContent), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write updated config: %v", err)
	}

	common.CopyFile(configPath, common.ConfigOutputPath)

	return nil
}

func (t *RelabelTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{
		verifyRelabeledMetrics(t.logGroupName),
		verifyMetricsInCloudWatch(t.namespace),
		verifyRelabeledMetricsInCloudWatch(t.namespace),
	}

	defer cleanup(t.logGroupName)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *RelabelTestRunner) GetTestName() string {
	return "Prometheus EMF Relabel Test"
}

func (t *RelabelTestRunner) GetAgentConfigFileName() string {
	return "emf_prometheus_relabel_config.json"
}

func (t *RelabelTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_summary_sum",
	}
}

func (t *RelabelTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func verifyRelabeledMetrics(logGroupName string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Relabeled Metrics",
		Status: status.FAILED,
	}

	streams := awsservice.GetLogStreams(logGroupName)
	if len(streams) == 0 {
		log.Printf("No log streams found in log group %s", logGroupName)
		return testResult
	}

	events, err := awsservice.GetLogsSince(logGroupName, *streams[0].LogStreamName, nil, nil)
	if err != nil {
		log.Printf("Failed to get log events: %v", err)
		return testResult
	}

	metricsFound := map[string]bool{
		"prometheus_test_counter":     false,
		"prometheus_test_gauge":       false,
		"prometheus_test_summary_sum": false,
	}

	for _, event := range events {
		var emfLog map[string]interface{}
		if err := json.Unmarshal([]byte(*event.Message), &emfLog); err != nil {
			log.Printf("Failed to parse EMF log: %v", err)
			continue
		}

		if _, ok := emfLog["CloudWatchMetrics"]; !ok {
			continue
		}

		myName, hasMyName := emfLog["my_name"].(string)
		myReplacementTest, hasMyReplacementTest := emfLog["my_replacement_test"].(string)
		promType, hasPromType := emfLog["prom_type"].(string)
		include, hasInclude := emfLog["include"].(string)
		if !hasMyName || !hasMyReplacementTest || !hasPromType || !hasInclude {
			log.Printf("EMF log missing my_name, my_replacement_test, prom_type, or include field: %v", emfLog)
			continue
		}

		// The test sets up a relabel config to combine the "include" and "promType" tags using
		// a replacement field that uses ${2} and $1 syntaxes. This verifies
		expectedMyReplacementTest := fmt.Sprintf("%s/%s", include, promType)
		if myReplacementTest != expectedMyReplacementTest {
			log.Printf("EMF log my_replacement_field is not as expected. expected %s, got %s. event %v", expectedMyReplacementTest, myReplacementTest, *event.Message)
			continue
		}

		if _, isTracked := metricsFound[myName]; isTracked {
			if _, hasMetric := emfLog[myName]; hasMetric {
				metricsFound[myName] = true
				log.Printf("Found correctly relabeled metric: %s", myName)
			}
		}
	}

	allMetricsFound := true
	missingMetrics := []string{}
	for metric, found := range metricsFound {
		if !found {
			allMetricsFound = false
			missingMetrics = append(missingMetrics, metric)
		}
	}

	if !allMetricsFound {
		log.Printf("Missing relabeled metrics: %v", missingMetrics)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
