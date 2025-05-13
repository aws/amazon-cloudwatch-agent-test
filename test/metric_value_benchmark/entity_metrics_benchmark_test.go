// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const entityNamespace = "EntityMetricValueBenchmarkTest"

type EntityMetricBenchmarkTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *EntityMetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting EntityMetricBenchmarkTestSuite")
}

func (suite *EntityMetricBenchmarkTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished EntityMetricBenchmarkTestSuite")
}

var entityTestRunners []*test_runner.TestRunner

func getEntityTestRunners(env *environment.MetaData) []*test_runner.TestRunner {

	// Only add entity related tests if in us-west-2 (we don't have access to ListEntitiesForMetric in CN/ITAR)
	if os.Getenv("AWS_REGION") == "us-west-2" && entityTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		entityTestRunners = []*test_runner.TestRunner{
			{TestRunner: &EntityMetricsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &StatsDEntityCustomServiceAndEnvironmentRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, make(chan bool)}},
			{TestRunner: &CollectDEntityCustomServiceAndEnvironmentRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectDEntityServiceAndEnvironmentFallback{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &StatsDEntityServiceAndEnvironmentFallback{test_runner.BaseTestRunner{DimensionFactory: factory}, make(chan bool)}},
		}
	}
	return entityTestRunners
}

func (suite *EntityMetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()

	// Entity tests are only supported in EC2 environment
	if env.ComputeType != computetype.EC2 {
		log.Println("Not in EC2, skipping entity metric tests")
		return
	}

	log.Println("Environment compute type is EC2")
	for _, testRunner := range getEntityTestRunners(env) {
		suite.AddToSuiteResult(testRunner.Run())
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Entity Metric Benchmark Test Suite Failed")
}

func (suite *EntityMetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestEntityMetricValueBenchmarkSuite(t *testing.T) {
	suite.Run(t, new(EntityMetricBenchmarkTestSuite))
}
