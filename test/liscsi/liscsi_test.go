// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package liscsi

import (
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type LISCSITestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *LISCSITestSuite) GetSuiteName() string {
	return "LIS CSI Container Insights"
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
				Runner: &LISCSITestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EKS_LIS_CSI", env},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *LISCSITestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	case computetype.EKS:
		log.Printf("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "LIS CSI Container Test Suite Failed")
}

func (suite *LISCSITestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestLISCSISuite(t *testing.T) {
	suite.Run(t, new(LISCSITestSuite))
}
