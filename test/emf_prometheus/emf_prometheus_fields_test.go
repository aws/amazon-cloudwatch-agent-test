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

//go:embed resources/prometheus.yaml
var fieldsPrometheusConfig string

//go:embed resources/prometheus_metrics
var fieldsPrometheusMetrics string

type EMFFieldsTestRunner struct {
	test_runner.BaseTestRunner
	namespace    string
	logGroupName string
}

func (t *EMFFieldsTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *EMFFieldsTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *EMFFieldsTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{
		verifyEMFFields(t.logGroupName),
		verifyMetricsInCloudWatch(t.namespace),
	}

	defer cleanup(t.logGroupName)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EMFFieldsTestRunner) GetTestName() string {
	return "Prometheus EMF Fields Test"
}

func (t *EMFFieldsTestRunner) GetAgentConfigFileName() string {
	return "emf_prometheus_fields_config.json"
}

func (t *EMFFieldsTestRunner) SetupBeforeAgentRun() error {
	instanceID := awsservice.GetInstanceId()
	t.namespace = fmt.Sprintf("%sfields_test_%s", namespacePrefix, instanceID)
	t.logGroupName = fmt.Sprintf("%sfields_test_%s", logGroupPrefix, instanceID)

	if err := setupPrometheus(fieldsPrometheusConfig, fieldsPrometheusMetrics, ""); err != nil {
		return err
	}

	configPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	updatedContent := strings.ReplaceAll(string(content), "${NAMESPACE}", t.namespace)
	updatedContent = strings.ReplaceAll(updatedContent, "${LOG_GROUP_NAME}", t.logGroupName)

	if err := os.WriteFile(configPath, []byte(updatedContent), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write updated config: %v", err)
	}
	common.CopyFile(configPath, common.ConfigOutputPath)

	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func verifyEMFFields(logGroupName string) status.TestResult {
	testResult := status.TestResult{
		Name:   "EMF Fields Presence",
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

	requiredFields := []string{"host", "instance", "job", "prom_metric_type"}
	expectedMetricTypes := map[string]bool{
		"counter": false,
		"gauge":   false,
		"summary": false,
	}

	for _, event := range events {
		log.Printf("Checking EMF log: %s", *event.Message)

		var emfLog map[string]interface{}
		if err := json.Unmarshal([]byte(*event.Message), &emfLog); err != nil {
			log.Printf("Failed to parse EMF log: %v", err)
			continue
		}

		missingFields := []string{}
		for _, field := range requiredFields {
			if _, ok := emfLog[field]; !ok {
				missingFields = append(missingFields, field)
			}
		}

		if len(missingFields) > 0 {
			log.Printf("EMF log missing required fields: %v", missingFields)
			continue
		}

		log.Printf("Found EMF fields - host: %s, instance: %s, job: %s, type: %s",
			emfLog["host"],
			emfLog["instance"],
			emfLog["job"],
			emfLog["prom_metric_type"])

		host, _ := emfLog["host"].(string)
		if !strings.Contains(host, ".internal") {
			log.Printf("Invalid host format: %s", host)
			return testResult
		}

		instance, _ := emfLog["instance"].(string)
		if instance != "localhost:8101" {
			log.Printf("Unexpected instance value: %s", instance)
			return testResult
		}

		job, _ := emfLog["job"].(string)
		if job != "prometheus_test_job" {
			log.Printf("Unexpected job value: %s", job)
			return testResult
		}

		metricType, _ := emfLog["prom_metric_type"].(string)
		if _, exists := expectedMetricTypes[metricType]; exists {
			expectedMetricTypes[metricType] = true
		}
	}

	missingTypes := []string{}
	for metricType, found := range expectedMetricTypes {
		if !found {
			missingTypes = append(missingTypes, metricType)
		}
	}

	if len(missingTypes) > 0 {
		log.Printf("Missing metric types in EMF logs: %v", missingTypes)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
