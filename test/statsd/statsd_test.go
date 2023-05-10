// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "StatsD"

type MetricBenchmarkTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting StatsDTestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished StatsDTestSuite")
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	ecsTestRunners []*test_runner.ECSTestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*test_runner.ECSTestRunner {
	if ecsTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		ecsTestRunners = []*test_runner.ECSTestRunner{
			{
				TestRunner:       &ECSStatsdTestRunner{test_runner.ECSBaseTestRunner{DimensionFactory: factory}},
				AgentRunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:              *env,
			},
		}
	}
	return ecsTestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	for _, ecsTestRunner := range getEcsTestRunners(env) {
		ecsTestRunner.Run(suite, env)
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Statsd Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestStatsdSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
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
	metricErrorBound := 0.2
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
