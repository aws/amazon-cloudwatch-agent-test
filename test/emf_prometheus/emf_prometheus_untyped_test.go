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
	randomSuffix := generateRandomSuffix()
	t.namespace = fmt.Sprintf("%suntyped_test_%s", namespacePrefix, randomSuffix)
	t.logGroupName = fmt.Sprintf("%suntyped_test_%s", logGroupPrefix, randomSuffix)

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
