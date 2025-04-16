package emf_prometheus

import (
	_ "embed"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//go:embed resources/prometheus.yaml
var untypedPrometheusConfig string

//go:embed resources/prometheus_metrics
var untypedPrometheusMetrics string

type UntypedTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *UntypedTestRunner) GetMeasuredMetrics() []string {
	return nil
}

func (t *UntypedTestRunner) Validate() status.TestGroupResult {
	randomSuffix := generateRandomSuffix()
	namespace := fmt.Sprintf("%s_untyped_test_%s", namespacePrefix, randomSuffix)
	logGroupName := fmt.Sprintf("%s_untyped_test_%s", logGroupPrefix, randomSuffix)

	testResults := []status.TestResult{}

	if err := setupPrometheus(untypedPrometheusConfig, untypedPrometheusMetrics, ""); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:   "Setup",
				Status: status.FAILED,
			}},
		}
	}

	if err := startAgent(filepath.Join("agent_configs", "emf_prometheus_untyped_config.json"), namespace, logGroupName); err != nil {
		return status.TestGroupResult{
			Name: t.GetTestName(),
			TestResults: []status.TestResult{{
				Name:   "Agent Start",
				Status: status.FAILED,
			}},
		}
	}

	defer cleanup(logGroupName)

	testResults = append(testResults, verifyUntypedMetricAbsence(namespace))
	testResults = append(testResults, verifyUntypedMetricLogsAbsence(logGroupName))
	testResults = append(testResults, verifyMetricsInCloudWatch(namespace))

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *UntypedTestRunner) GetTestName() string {
	return "Prometheus EMF Untyped Test"
}

func (t *UntypedTestRunner) GetAgentConfigFileName() string {
	return filepath.Join("agent_configs", "emf_prometheus_untyped_config.json")
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
