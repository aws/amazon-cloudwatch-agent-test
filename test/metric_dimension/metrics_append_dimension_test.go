// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"

	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"log"
	"testing"
)

type MetricsAppendDimensionTestSuite struct {
	suite.Suite
	result status.TestSuiteResult
}

func (suite *MetricsAppendDimensionTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricAppendDimensionTestSuite")
}

func (suite *MetricsAppendDimensionTestSuite) TearDownSuite() {
	suite.result.Print()
	fmt.Println(">>>> Finished MetricAppendDimensionTestSuite")
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	testRunners []*test_runner.TestRunner
)

func getTestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if testRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		testRunners = []*test_runner.TestRunner{
			{TestRunner: &NoAppendDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &OneAggregateDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return testRunners
}

func (suite *MetricsAppendDimensionTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	for _, testRunner := range getTestRunners(env) {
		testRunner.Run(suite)
	}

	suite.Assert().Equal(status.SUCCESSFUL, status.FAILED, "Metric Append Dimension Test Suite Failed")

	//suite.Assert().Equal(status.SUCCESSFUL, suite.result.GetStatus(), "Metric Append Dimension Test Suite Failed")
}

func (suite *MetricsAppendDimensionTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.result.TestGroupResults = append(suite.result.TestGroupResults, r)
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
