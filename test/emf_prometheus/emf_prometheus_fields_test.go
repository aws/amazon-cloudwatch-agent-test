// emf_fields_runner.go
package emf_prometheus

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"log"
	"path/filepath"
)

//go:embed resources/prometheus.yaml
var fieldsPrometheusConfig string

//go:embed resources/prometheus_metrics
var fieldsPrometheusMetrics string

type EMFFieldsTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *EMFFieldsTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *EMFFieldsTestRunner) Validate() status.TestGroupResult {
	randomSuffix := generateRandomSuffix()
	namespace := fmt.Sprintf("%s_fields_test_%s", namespacePrefix, randomSuffix)
	logGroupName := fmt.Sprintf("%s_fields_test_%s", logGroupPrefix, randomSuffix)

	if err := setupPrometheus(fieldsPrometheusConfig, fieldsPrometheusMetrics, ""); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:    "Setup",
				Status:  status.FAILED,
			}},
		}
	}

	if err := startAgent(filepath.Join("agent_configs", "emf_prometheus_fields_config.json"), namespace, logGroupName); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:    "Agent Start",
				Status:  status.FAILED,
			}},
		}
	}

	defer cleanup(logGroupName)

	testResults := []status.TestResult{
		verifyEMFFields(logGroupName),
		verifyMetricsInCloudWatch(namespace),
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EMFFieldsTestRunner) GetTestName() string {
	return "Prometheus EMF Fields Test"
}

func (t *EMFFieldsTestRunner) GetAgentConfigFileName() string {
	return filepath.Join("agent_configs", "emf_prometheus_fields_config.json")
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

		// Log field values
		log.Printf("Found EMF fields - host: %s, instance: %s, job: %s, type: %s",
			emfLog["host"],
			emfLog["instance"],
			emfLog["job"],
			emfLog["prom_metric_type"])

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