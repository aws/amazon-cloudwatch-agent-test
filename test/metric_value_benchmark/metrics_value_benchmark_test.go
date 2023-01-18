// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows
// +build !windows

package metric_value_benchmark

import (
	"fmt"
	"log"
	"strings"
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
	result status.TestSuiteResult
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricBenchmarkTestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.result.Print()
	fmt.Println(">>>> Finished MetricBenchmarkTestSuite")
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	ecsTestRunners []*ECSTestRunner
	ec2TestRunners []*test_runner.TestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*ECSTestRunner {
	if ecsTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		ecsTestRunners = []*ECSTestRunner{
			{
				testRunner:       &ContainerInsightsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}},
				agentRunStrategy: &ECSAgentRunStrategy{},
				env:              *env,
			},
		}
	}
	return ecsTestRunners
}

func getEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if ec2TestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		ec2TestRunners = []*test_runner.TestRunner{
			{TestRunner: &DiskTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &NetStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &PrometheusTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CPUTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &MemTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &ProcStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &DiskIOTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &NetTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &StatsdTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &EMFTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectDTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &SwapTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &ProcessesTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return ec2TestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	if env.ComputeType == computetype.ECS {
		log.Print("Environment compute type is ECS")
		for _, ecsTestRunner := range getEcsTestRunners(env) {
			ecsTestRunner.Run(suite, env)
		}
	} else {
		for _, testRunner := range getEc2TestRunners(env) {
			if shouldRunEC2Test(env, testRunner) {
				testRunner.Run(suite)
			}
		}
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.result.GetStatus(), "Metric Benchmark Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.result.TestGroupResults = append(suite.result.TestGroupResults, r)
}

func TestMetricValueBenchmarkSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
}

func shouldRunEC2Test(env *environment.MetaData, t *test_runner.TestRunner) bool {
	if env.EC2PluginTests == nil {
		return true // default behavior is to run all tests
	}
	_, ok := env.EC2PluginTests[strings.ToLower(t.TestRunner.GetTestName())]
	return ok
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
