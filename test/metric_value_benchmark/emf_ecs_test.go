package metric_value_benchmark

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"time"
)

type EMFECSTestRunner struct {
	test_runner.BaseTestRunner
}

var _ IECSTestRunner = (*EMFECSTestRunner)(nil)

func (t *EMFECSTestRunner) validate(e *environment.MetaData) status.TestGroupResult {
	metricsToFetch := t.getMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEMFMetric(metricName)
	}

	testResults = append(testResults, validateEMFLogs("MetricValueBenchmarkTest", awsservice.GetInstanceId()))

	return status.TestGroupResult{
		Name:        t.getTestName(),
		TestResults: testResults,
	}
}

func (t *EMFECSTestRunner) getTestName() string {
	return "EMFOnECS"

}

func (t *EMFECSTestRunner) getAgentConfigFileName() string {
	return "./agent_configs/emf_config.json"
}

func (t *EMFECSTestRunner) getAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *EMFECSTestRunner) getMeasuredMetrics() []string {
	return []string{"EMFCounter"}
}

func (t *EMFECSTestRunner) validateEMFMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "Type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("Counter")},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 5) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
