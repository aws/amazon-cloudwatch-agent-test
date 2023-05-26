// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package statsd

import (
	"fmt"
	"log"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const namespace = "Fluent"

type FluentTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *FluentTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting FluentTestSuite")
}

func (suite *FluentTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished FluentTestSuite")
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

var (
	eksTestRunners []*test_runner.EKSTestRunner
)

func getEksTestRunners(env *environment.MetaData) []*test_runner.EKSTestRunner {
	if eksTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		eksTestRunners = []*test_runner.EKSTestRunner{
			{
				Runner: &FluentRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, fmt.Sprintf("%s/%s", namespace, env.ComputeType)},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *FluentTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData(envMetaDataStrings)
	switch env.ComputeType {
	case computetype.EKS:
		log.Println("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Fluent Test Suite Failed")
}

func (suite *FluentTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestFluentSuite(t *testing.T) {
	suite.Run(t, new(FluentTestSuite))
}
