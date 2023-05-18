package emf_ecs

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"log"
	"time"
)

type EMFECSTestRunner struct {
	test_runner.BaseTestRunner
}

func (t *EMFECSTestRunner) Validate(e *environment.MetaData) status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateEMFOnECSMetrics(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EMFECSTestRunner) GetTestName() string {
	return "EMFOnECS"

}

func (t *EMFECSTestRunner) GetAgentConfigFileName() string {
	return "./resources/config.json"
}

func (t *EMFECSTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *EMFECSTestRunner) GetMeasuredMetrics() []string {
	return []string{"EMFCounter"}
}

func (t *EMFECSTestRunner) validateEMFOnECSMetrics(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch("EMFNameSpace", metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	// TODO: Range test with >0 and <100
	// TODO: Range test: which metric to get? api reference check. should I get average or test every single datapoint for 10 minutes? (and if 90%> of them are in range, we are good)

	testResult.Status = status.SUCCESSFUL
	return testResult
}

// isAllValuesGreaterThanOrEqualToExpectedValue will compare if the given array is larger than 0
// and check if the average value for the array is not la
// TODO: Moving metric_value_benchmark to validator
// https://github.com/aws/amazon-cloudwatch-agent-test/pull/162
func isAllValuesGreaterThanOrEqualToExpectedValue(metricName string, values []float64, expectedValue float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}

	totalSum := 0.0
	for _, value := range values {
		if value < 0 {
			log.Printf("Values are not all greater than or equal to zero for %s", metricName)
			return false
		}
		totalSum += value
	}
	metricErrorBound := 0.1
	metricAverageValue := totalSum / float64(len(values))
	upperBoundValue := expectedValue * (1 + metricErrorBound)
	lowerBoundValue := expectedValue * (1 - metricErrorBound)
	if expectedValue > 0 && (metricAverageValue > upperBoundValue || metricAverageValue < lowerBoundValue) {
		log.Printf("The average value %f for metric %s are not within bound [%f, %f]", metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
		return false
	}

	log.Printf("The average value %f for metric %s are within bound [%f, %f]", expectedValue, metricName, lowerBoundValue, upperBoundValue)
	return true
}
