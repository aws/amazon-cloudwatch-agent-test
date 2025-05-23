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

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var untypedPrometheusConfig string

//go:embed resources/prometheus_metrics
var untypedPrometheusMetrics string

type UntypedTestRunner struct {
	test_runner.BaseTestRunner
	namespace    string
	logGroupName string
}

func (t *UntypedTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *UntypedTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{
		verifyUntypedMetricAbsence(t.namespace),
		verifyUntypedMetricLogsAbsence(t.logGroupName),
		verifyMetricsInCloudWatch(t.namespace),
	}
	defer cleanup(t.logGroupName)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *UntypedTestRunner) GetTestName() string {
	return "Prometheus EMF Untyped Test"
}

func (t *UntypedTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *UntypedTestRunner) GetAgentConfigFileName() string {
	return "emf_prometheus_untyped_config.json"
}
func (t *UntypedTestRunner) SetupBeforeAgentRun() error {
	instanceID := awsservice.GetInstanceId()
	t.namespace = fmt.Sprintf("%suntyped_test_%s", namespacePrefix, instanceID)
	t.logGroupName = fmt.Sprintf("%suntyped_test_%s", logGroupPrefix, instanceID)

	if err := setupPrometheus(untypedPrometheusConfig, untypedPrometheusMetrics, ""); err != nil {
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

func verifyUntypedMetricAbsence(namespace string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Untyped Metric Absence",
		Status: status.FAILED,
	}

	dims := []types.Dimension{
		{
			Name:  aws.String("prom_type"),
			Value: aws.String("untyped"),
		},
	}

	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(namespace, "prometheus_test_untyped", dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
	if err != nil {
		log.Printf("Error fetching untyped metric: %v", err)
	}
	if len(values) > 0 {
		log.Printf("Found untyped metric when it should have been filtered out. Values: %v", values)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func verifyUntypedMetricLogsAbsence(logGroupName string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Untyped Metric Logs Absence",
		Status: status.FAILED,
	}

	streams := awsservice.GetLogStreams(logGroupName)
	if len(streams) == 0 {

		return testResult
	}

	events, err := awsservice.GetLogsSince(logGroupName, *streams[0].LogStreamName, nil, nil)
	if err != nil {
		return testResult
	}

	for _, event := range events {
		if strings.Contains(*event.Message, "prometheus_test_untyped") {
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func verifyHistogramMetrics(namespace string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Histogram Metrics Verification",
		Status: status.FAILED,
	}

	dims := []types.Dimension{
		{
			Name:  aws.String("prom_type"),
			Value: aws.String("histogram"),
		},
	}

	valueFetcher := metric.MetricValueFetcher{}

	// Check histogram sum
	sumValues, err := valueFetcher.Fetch(namespace, "prometheus_test_histogram_sum", dims, metric.AVERAGE, metric.MinuteStatPeriod)
	if err != nil || len(sumValues) == 0 {
		log.Printf("Error fetching histogram sum metric: %v", err)
		return testResult
	}
	if sumValues[0] != 300 {
		log.Printf("Unexpected histogram sum value: %v", sumValues[0])
		return testResult
	}

	// Check histogram count
	countValues, err := valueFetcher.Fetch(namespace, "prometheus_test_histogram_count", dims, metric.AVERAGE, metric.MinuteStatPeriod)
	if err != nil || len(countValues) == 0 {
		log.Printf("Error fetching histogram count metric: %v", err)
		return testResult
	}
	if countValues[0] != 75 {
		log.Printf("Unexpected histogram count value: %v", countValues[0])
		return testResult
	}

	// Check histogram buckets
	buckets := map[string]float64{
		"0":    1,
		"0.5":  2,
		"2.5":  3,
		"5":    4,
		"+Inf": 5,
	}

	for le, expectedValue := range buckets {
		bucketDims := append(dims, types.Dimension{
			Name:  aws.String("le"),
			Value: aws.String(le),
		})

		bucketValues, err := valueFetcher.Fetch(namespace, "prometheus_test_histogram_bucket", bucketDims, metric.AVERAGE, metric.MinuteStatPeriod)
		if err != nil || len(bucketValues) == 0 {
			log.Printf("Error fetching histogram bucket metric (le=%s): %v", le, err)
			return testResult
		}
		if bucketValues[0] != expectedValue {
			log.Printf("Unexpected histogram bucket value for le=%s: got %v, want %v", le, bucketValues[0], expectedValue)
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
