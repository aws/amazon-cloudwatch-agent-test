// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	"log"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

const namespace = "MetricAppendDimensionTest"

func TestMetricsAppendDimensionTestSuite(t *testing.T) {
	suite.Run(t, new(MetricsAppendDimensionTestSuite))
}

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
	ec2TestRunners []*TestRunner
)

func getEc2TestRunners(env *environment.MetaData) []*TestRunner {
	if ec2TestRunners == nil {
		factory := &metric.MetricFetcherFactory{Env: env}
		ec2TestRunners = []*metric_value_benchmark.TestRunner{
			{testRunner: &NoAppendDimensionTestRunner{BaseTestRunner{MetricFetcherFactory: factory}}},
		}
	}
	return ec2TestRunners
}

func (suite *MetricsAppendDimensionTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	for _, testRunner := range getEc2TestRunners(env) {
		testRunner.Run(suite)
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.result.GetStatus(), "Metric Append Dimension Test Suite Failed")
}

func (suite *MetricsAppendDimensionTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.result.TestGroupResults = append(suite.result.TestGroupResults, r)
}

func isValuesExist(metricName string, values []float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}
	return true
}
