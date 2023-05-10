// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf_ecs

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"testing"
)

const namespace = "EMF"

type MetricBenchmarkTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting EMF_ECS TestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished EMF_ECS TestSuite")
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
				TestRunner:       &EMFECSTestRunner{test_runner.ECSBaseTestRunner{DimensionFactory: factory}},
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

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "EMF_ECS Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestEMFSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
}
