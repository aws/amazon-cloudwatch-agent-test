// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package security

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"testing"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

type SecurityTestSuite struct {
	suite.Suite
	result status.TestSuiteResult
}

func (suite *SecurityTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting SecurityTestSuite")
}

func (suite *SecurityTestSuite) TearDownSuite() {
	suite.result.Print()
	fmt.Println(">>>> Finished SecurityTestSuite")
}

var (
	ec2TestRunners []*test_runner.TestRunner
)

/*
func getEc2TestRunners() []*test_runner.TestRunner {
	if ec2TestRunners == nil {
		ec2TestRunners = []*test_runner.TestRunner{
			{TestRunner: &FilePermissionTestRunner{}},
		}
	}
	return ec2TestRunners
}*/

func (suite *SecurityTestSuite) TestAllInSuite() {
	/*
		for _, testRunner := range getEc2TestRunners() {
			testRunner.Run(suite)
		}*/

	suite.Assert().Equal(status.SUCCESSFUL, status.FAILED, "Security Test Suite Failed")
	//suite.Assert().Equal(status.SUCCESSFUL, suite.result.GetStatus(), "Security Test Suite Failed")
}

func (suite *SecurityTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.result.TestGroupResults = append(suite.result.TestGroupResults, r)
}

func TestSecurityTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityTestSuite))
}
