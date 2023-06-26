// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"fmt"
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
)

type MetricsAppendDimensionTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricsAppendDimensionTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricAppendDimensionTestSuite")
}

func (suite *MetricsAppendDimensionTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished MetricAppendDimensionTestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	testRunners []*test_runner.TestRunner
)

func getTestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if testRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		testRunners = []*test_runner.TestRunner{
			{TestRunner: &NoAppendDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &GlobalAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &OneAggregateDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &AggregationDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return testRunners
}

func (suite *MetricsAppendDimensionTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	for _, testRunner := range getTestRunners(env) {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Metric Append Dimension Test Suite Failed")
}

func (suite *MetricsAppendDimensionTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestMetricsAppendDimensionTestSuite(t *testing.T) {
	suite.Run(t, new(MetricsAppendDimensionTestSuite))
}

func isAllValuesGreaterThanOrEqualToZero(metricName string, values []float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}
	for _, value := range values {
		if value < 0 {
			log.Printf("Values are not all greater than or equal to zero for %v", metricName)
			return false
		}
	}
	log.Printf("Values are all greater than or equal to zero for %v", metricName)
	return true
}
