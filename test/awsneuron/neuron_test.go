// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type AwsNeuronTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *AwsNeuronTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting AWS Neuron Container Insights TestSuite")
}

func (suite *AwsNeuronTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished AWS Neuron Container Insights TestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	eksTestRunners []*test_runner.EKSTestRunner
)

func getEksTestRunners(env *environment.MetaData) []*test_runner.EKSTestRunner {
	if eksTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		eksTestRunners = []*test_runner.EKSTestRunner{
			{
				Runner: &AwsNeuronTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EKS_AWS_NEURON", env},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *AwsNeuronTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	case computetype.EKS:
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "AWS Neuron Container Test Suite Failed")
}

func (suite *AwsNeuronTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestAWSNeuronSuite(t *testing.T) {
	suite.Run(t, new(AwsNeuronTestSuite))
}
