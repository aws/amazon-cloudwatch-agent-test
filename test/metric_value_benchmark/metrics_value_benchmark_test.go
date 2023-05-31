// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksdeploymenttype"
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
	ecsTestRunners []*test_runner.ECSTestRunner
	ec2TestRunners []*test_runner.TestRunner
	eksTestRunners []*test_runner.EKSTestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*test_runner.ECSTestRunner {
	if ecsTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		ecsTestRunners = []*test_runner.ECSTestRunner{
			{
				Runner: &ContainerInsightsTestRunner{
					BaseTestRunner: test_runner.BaseTestRunner{DimensionFactory: factory},
					env:            env,
				},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
		}
	}
	return ecsTestRunners
}

func getEksTestRunners(env *environment.MetaData) []*test_runner.EKSTestRunner {
	if eksTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		switch env.EksDeploymentStrategy {
		case eksdeploymenttype.DAEMON:
			eksDaemonTestRunner := test_runner.EKSTestRunner{
				Runner: &EKSDaemonTestRunner{BaseTestRunner: test_runner.BaseTestRunner{
					DimensionFactory: factory,
				},
					env: env,
				},
				Env: *env,
			}
			eksTestRunners = append(eksTestRunners, &eksDaemonTestRunner)
		case eksdeploymenttype.REPLICA:
			eksDeploymentTestRunner := test_runner.EKSTestRunner{
				Runner: &EKSDeploymentTestRunner{BaseTestRunner: test_runner.BaseTestRunner{
					DimensionFactory: factory,
				},
					env: env,
				},
				Env: *env,
			}
			eksTestRunners = append(eksTestRunners, &eksDeploymentTestRunner)
		}
	}
	return eksTestRunners
}

func getEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if ec2TestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		ec2TestRunners = []*test_runner.TestRunner{
			{TestRunner: &StatsdTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &DiskTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &NetStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &PrometheusTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CPUTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &MemTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &ProcStatTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &DiskIOTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &NetTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &EthtoolTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &EMFTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &SwapTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &ProcessesTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectDTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &RenameSSMTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return ec2TestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	switch env.ComputeType {
	case computetype.ECS:
		log.Println("Environment compute type is ECS")
		for _, ecsTestRunner := range getEcsTestRunners(env) {
			ecsTestRunner.Run(suite, env)
		}
	case computetype.EKS:
		log.Println("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default: // EC2 tests
		log.Println("Environment compute type is EC2")
		for _, testRunner := range getEc2TestRunners(env) {
			if shouldRunEC2Test(env, testRunner) {
				testRunner.Run(suite)
			}
		}
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Metric Benchmark Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
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
