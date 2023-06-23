// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"log"
	"testing"
)

type MetricBenchmarkTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricBenchmarkTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting EMF Container TestSuite")
}

func (suite *MetricBenchmarkTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished EMF Container TestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	ecsTestRunners []*test_runner.ECSTestRunner
	eksTestRunners []*test_runner.EKSTestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*test_runner.ECSTestRunner {
	if ecsTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		ecsTestRunners = []*test_runner.ECSTestRunner{
			{
				Runner:      &EMFTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EMF_ECS"},
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

		eksTestRunners = []*test_runner.EKSTestRunner{
			{
				Runner: &EMFTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EMF_EKS"},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *MetricBenchmarkTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
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
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "EMF Container Test Suite Failed")
}

func (suite *MetricBenchmarkTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestEMFSuite(t *testing.T) {
	suite.Run(t, new(MetricBenchmarkTestSuite))
}
