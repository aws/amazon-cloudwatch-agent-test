//go:build !windows

package emf

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"time"
)

type EMFTestRunner struct {
	test_runner.BaseTestRunner
	testName string
}

var _ test_runner.ITestRunner = (*EMFTestRunner)(nil)

func (t *EMFTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEMFMetrics(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EMFTestRunner) GetTestName() string {
	return "EMF Container Tests"

}

func (t *EMFTestRunner) GetAgentConfigFileName() string {
	return "./resources/config.json"
}

func (t *EMFTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *EMFTestRunner) GetMeasuredMetrics() []string {
	return []string{"EMFCounter"}
}

func (t *EMFTestRunner) validateEMFMetrics(metricName string) status.TestResult {
	namespace := ""
	var dims []types.Dimension
	var failed []dimension.Instruction
	if t.testName == "EMF_ECS" {
		namespace = "EMFECSNameSpace"
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "InstanceID",
				Value: dimension.UnknownDimensionValue(),
			},
			{
				Key:   "Type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("Counter")},
			},
		})
	}
	if t.testName == "EMF_EKS" {
		namespace = "EMFEKSNameSpace"
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{})
	}
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}
