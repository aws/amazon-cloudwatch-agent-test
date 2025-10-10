// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ebscsi

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type EBSCSITestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *EBSCSITestSuite) SetupSuite() {
	fmt.Println(">>>> Starting EBS Container Insights TestSuite")
}

func (suite *EBSCSITestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished EBS Container Insights TestSuite")
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
				Runner: &DiskIOTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EKS_EBS_DISKIO", env},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *EBSCSITestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	case computetype.EKS:
		log.Println("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "EBS Container Test Suite Failed")
}

func (suite *EBSCSITestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestEBSCSISuite(t *testing.T) {
	suite.Run(t, new(EBSCSITestSuite))
}
