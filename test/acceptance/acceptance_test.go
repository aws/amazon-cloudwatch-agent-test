// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package acceptance

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/acceptance/testrunners"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"testing"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

type AcceptanceTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

var _ test_runner.ITestSuite = (*AcceptanceTestSuite)(nil)

func (suite *AcceptanceTestSuite) GetSuiteName() string {
	return "AcceptanceTestSuite"
}

func getEc2TestRunners() []*test_runner.TestRunner {
	return []*test_runner.TestRunner{
		{TestRunner: &testrunners.FilePermissionTestRunner{}},
	}
}

func (suite *AcceptanceTestSuite) TestAllInSuite() {
	for _, testRunner := range getEc2TestRunners() {
		testRunner.Run(suite)
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Security Test Suite Failed")
}

func TestAcceptanceTestSuite(t *testing.T) {
	suite.Run(t, new(AcceptanceTestSuite))
}
