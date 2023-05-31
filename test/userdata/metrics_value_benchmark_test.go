// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "MetricValueBenchmarkTest"

type MetricBenchmarkTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricBenchmarkTestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished MetricBenchmarkTestSuite")
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	ec2TestRunners []*test_runner.TestRunner
)

func getEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if ec2TestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		ec2TestRunners = []*test_runner.TestRunner{
			{TestRunner: &UserdataTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return ec2TestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	switch env.ComputeType {
	case computetype.EC2: // EC2 tests
		log.Println("Environment compute type is EC2")
		for _, testRunner := range getEc2TestRunners(env) {
			testRunner.Run(suite)
		}
	default:
		log.Println("Invalid environment being used")
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Metric Benchmark Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestMetricValueBenchmarkSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
}