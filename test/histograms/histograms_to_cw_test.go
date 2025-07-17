// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package histograms

import (
	_ "embed"
	"fmt"
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

var metadata *environment.MetaData

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &OtlpHistogramTestRunner{},
		},
	}
)

var _ test_runner.ITestRunner = (*OtlpHistogramTestRunner)(nil)

const (
	namespace = "CWAgent/OTLPHistograms"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestHistogramTestSuite(t *testing.T) {
	suite.Run(t, &HistogramTestSuite{})
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
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Histogram to CW Test Suite Failed")
}

type OtlpHistogramTestRunner struct {
	test_runner.BaseTestRunner
}

func (t OtlpHistogramTestRunner) GetTestName() string {
	return "otlp_histograms"
}

func (t OtlpHistogramTestRunner) GetAgentConfigFileName() string {
	return "otlp_config.json"
}

func (t OtlpHistogramTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{}
	testResults = append(testResults, t.validateHistogramMetric("my.delta.histogram")...)
	testResults = append(testResults, t.validateHistogramMetric("my.cumulative.histogram")...)
	testResults = append(testResults, t.validateHistogramMetric("my.delta.exponential.histogram")...)
	testResults = append(testResults, t.validateHistogramMetric("my.cumulative.exponential.histogram")...)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *OtlpHistogramTestRunner) GetAgentRunDuration() time.Duration {
	return 5 * time.Minute
}

func (t OtlpHistogramTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
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

func (t OtlpHistogramTestRunner) SetupAfterAgentRun() error {
	// OTLP source has some special setup after the agent starts
	return common.RunAsyncCommand("resources/otlp_pusher.sh")
}

func (t *OtlpHistogramTestRunner) validateHistogramMetric(metricName string) []status.TestResult {
	results := []status.TestResult{}

	dims := getDimensions(metadata.InstanceId)
	if len(dims) == 0 {
		reason := fmt.Errorf("unable to determine dimensions for %s", metricName)
		log.Println(reason)
		return []status.TestResult{{
			Name:   metricName,
			Status: status.FAILED,
			Reason: reason,
		}}
	}

	a := map[string][]struct {
		stat    types.Statistic
		value   float64
		epsilon float64
	}{
		"my.delta.histogram": {
			{
				stat:  types.StatisticMinimum,
				value: 0,
			},
			{
				stat:  types.StatisticMaximum,
				value: 2,
			},
			{
				stat:  types.StatisticSum,
				value: 24,
			},
			{
				stat:  types.StatisticAverage,
				value: 2,
			},
			{
				stat:  types.StatisticSampleCount,
				value: 12,
			},
		},
		"my.cumulative.histogram": {
			{
				stat:  types.StatisticMinimum,
				value: 0,
			},
			{
				stat:  types.StatisticMaximum,
				value: 2,
			},
			{
				stat:  types.StatisticSum,
				value: 24,
			},
			{
				stat:  types.StatisticAverage,
				value: 2,
			},
			{
				stat:  types.StatisticSampleCount,
				value: 12,
			},
		},
		"my.delta.exponential.histogram": {
			{
				stat:  types.StatisticMinimum,
				value: 0,
			},
			{
				stat:  types.StatisticMaximum,
				value: 5,
			},
			{
				stat:  types.StatisticSum,
				value: 60, // we send Sum=10 six times in one minute
			},
			{
				stat:  types.StatisticAverage,
				value: 3.33, // sum / samplecount = 10/3
			},
			{
				stat:  types.StatisticSampleCount,
				value: 18, // we send Count=3 six times in one minute
			},
		},
		"my.cumulative.exponential.histogram": {
			{
				stat:  types.StatisticMinimum,
				value: 0, // min/max are invalid for cumulative histograms converted to delta
			},
			{
				stat:  types.StatisticMaximum,
				value: 0, // min/max are invalid for cumulative histograms converted to delta
			},
			{
				stat:  types.StatisticSum,
				value: 12, // we send Sum=2 six times in one minute
			},
			{
				stat:  types.StatisticAverage,
				value: 1, // sum / samplecount = 2/2
			},
			{
				stat:  types.StatisticSampleCount,
				value: 12, // we send Count=2 six times in one minute
			},
		},
	}
	expected, ok := a[metricName]
	if !ok {
		reason := fmt.Errorf("Unable to determine expected metrics for %s\n", metricName)
		log.Println(reason)
		return []status.TestResult{{
			Name:   metricName,
			Status: status.FAILED,
			Reason: reason,
		}}
	}
	fetcher := metric.MetricValueFetcher{}
	for _, e := range expected {
		testResult := status.TestResult{
			Name:   fmt.Sprintf("%s/%s", metricName, e.stat),
			Status: status.FAILED,
		}
		values, err := fetcher.Fetch(namespace, metricName, dims, metric.Statistics(e.stat), metric.MinuteStatPeriod)
		if err != nil {
			reason := fmt.Errorf("Unable to fetch metrics for namespace {%s} metric name {%s} stat {%s}: %w", namespace, metricName, e.stat, err)
			log.Println(reason)
			testResult.Reason = reason
			results = append(results, testResult)
			continue
		}
		if len(values) < 3 {
			reason := fmt.Errorf("Not enough metrics retrieved for namespace {%s} metric Name {%s} stat {%s}. Need at least 3, got %d", namespace, metricName, e.stat, len(values))
			log.Println(reason)
			testResult.Reason = reason
			results = append(results, testResult)
			continue
		}
		// omit first/last metric as the 1m collection intervals may be missing data points from when the agent was started/stopped
		if err := metric.IsAllValuesGreaterThanOrEqualToExpectedValueWithError(metricName, values[1:len(values)-1], e.value); err != nil {
			testResult.Reason = err
			results = append(results, testResult)
			continue
		}

		testResult.Status = status.SUCCESSFUL
		results = append(results, testResult)
	}

	return results
}
