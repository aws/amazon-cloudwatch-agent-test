// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "StatsD"

type StatsDTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *StatsDTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting StatsDTestSuite")
}

func (suite *StatsDTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished StatsDTestSuite")
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
				Runner:      &StatsDRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, fmt.Sprintf("%s/%s", namespace, env.ComputeType), "InstanceId"},
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
				Runner: &StatsDRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, fmt.Sprintf("%s/%s", namespace, env.ComputeType), "ClusterName"},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *StatsDTestSuite) TestAllInSuite() {
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

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Statsd Test Suite Failed")
}

func (suite *StatsDTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestStatsdSuite(t *testing.T) {
	suite.Run(t, new(StatsDTestSuite))
}
