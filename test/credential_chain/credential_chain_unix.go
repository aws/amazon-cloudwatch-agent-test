// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package credential_chain

import (
	"log"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

var metadata *environment.MetaData

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type CredentialChainTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *CredentialChainTestSuite) SetupSuite() {
	log.Println(">>>> Starting CredentialChainTestSuite")
}

func (suite *CredentialChainTestSuite) TearDownSuite() {
	suite.Result.Print()
	log.Println(">>>> Finished CredentialChainTestSuite")
}

var (
	testRunners = []*test_runner.TestRunner{
		{
			TestRunner: &CommonConfigTestRunner{
				BaseTestRunner: test_runner.BaseTestRunner{},
			},
		},
		{
			TestRunner: &HomeEnvTestRunner{
				BaseTestRunner: test_runner.BaseTestRunner{},
			},
		},
	}
)

func (suite *CredentialChainTestSuite) TestAllInSuite() {
	metadata = environment.GetEnvironmentMetaData()
	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Credential Chain Test Suite Failed")
}
