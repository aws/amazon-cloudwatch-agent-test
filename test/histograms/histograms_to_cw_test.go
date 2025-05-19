// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package amp

import (
	_ "embed"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

// NOTE: this should match with append_dimensions under metrics in agent config
var appendDims = map[string]string{
	"d1": "foo",
	"d2": "bar",
}

var metadata *environment.MetaData

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &HistogramTestRunner{},
		},
	}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestHistogramTestSuite(t *testing.T) {
	suite.Run(t, new(HistogramTestSuite))
}

type HistogramTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *HistogramTestSuite) GetSuiteName() string {
	return "Histogram to CW"
}

func (suite *HistogramTestSuite) TestAllInSuite() {
	metadata = environment.GetEnvironmentMetaData()

	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Assume Role Test Suite Failed")
}

type HistogramTestRunner struct {
	test_runner.BaseTestRunner
}

func (t HistogramTestRunner) GetTestName() string {
	return "otlp_histograms"
}

func (t HistogramTestRunner) GetAgentConfigFileName() string {
	return "otlp_config.json"
}

func (t HistogramTestRunner) Validate() status.TestGroupResult {
	return t.validateOtlpHistogramMetrics()
}

func (t *HistogramTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *HistogramTestRunner) validateOtlpHistogramMetrics() status.TestGroupResult {
	histogramMetrics := t.getOtlpHistogramMetrics()
	testResults := make([]status.TestResult, len(histogramMetrics))

	for i, metricName := range histogramMetrics {
		testResults[i] = t.validateHistogramMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *HistogramTestRunner) validateHistogramMetric(metricName string) status.TestResult {
	namespace := "CWAgent/OTLPHistograms"

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims := getDimensions(metadata.InstanceId)
	if len(dims) == 0 {
		log.Printf("Unable to determine dimensions for %s\n", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, "Maximum", metric.HighResolutionStatPeriod)
	if err != nil {
		log.Printf("Unable to fetch metrics for namespace {%s} metric name {%s} dims: {%v}\n", namespace, metricName, dims)
		return testResult
	}

	log.Printf("Metrics retrieved from cloudwatch for Metric Name {%s} are: %v\n", metricName, values)

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t HistogramTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}

func (t HistogramTestRunner) getOtlpHistogramMetrics() []string {
	return []string{
		"my.cumulative.histogram",
		"my.delta.histogram",
	}
}

func getDimensions(metricName string) []types.Dimension {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceType",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return []types.Dimension{}
	}

	return dims
}

func (t HistogramTestRunner) SetupAfterAgentRun() error {
	// OTLP source has some special setup after the agent starts
	return common.RunAsyncCommand("resources/otlp_pusher.sh")
}

var _ test_runner.ITestRunner = (*HistogramTestRunner)(nil)
