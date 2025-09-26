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
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
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

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	ecsTestRunners []*test_runner.ECSTestRunner
	ec2TestRunners []*test_runner.TestRunner
	eksTestRunners []*test_runner.EKSTestRunner
)




func getEc2TestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if ec2TestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		ec2TestRunners = []*test_runner.TestRunner{}

		// Only add the Disk IO EBS and Instance Store test if not running on SELinux
		runningOnSELinux, _ := common.SELinuxEnforced()
		if !runningOnSELinux {
			ec2TestRunners = append(ec2TestRunners,
				&test_runner.TestRunner{TestRunner: &DiskIOInstanceStoreTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			)
		}
	}
	return ec2TestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	default: // EC2 tests
		log.Println("Environment compute type is EC2")
		for _, testRunner := range getEc2TestRunners(env) {
			if shouldRunEC2Test(env, testRunner) {
				suite.AddToSuiteResult(testRunner.Run())
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
	if env.EC2PluginTests == nil && env.ExcludedTests == nil {
		return true // default behavior is to run all tests
	}
	_, shouldRun := env.EC2PluginTests[strings.ToLower(t.TestRunner.GetTestName())]
	_, shouldExclude := env.ExcludedTests[strings.ToLower(t.TestRunner.GetTestName())]
	if shouldRun {
		return true
	} else if len(env.ExcludedTests) != 0 {
		return !shouldExclude
	}
	return false
}
