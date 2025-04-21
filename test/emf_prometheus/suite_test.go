// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf_prometheus

import (
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type PrometheusEMFTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *PrometheusEMFTestSuite) SetupSuite() {
	log.Println(">>>> Starting Prometheus EMF TestSuite")
}

func (suite *PrometheusEMFTestSuite) TearDownSuite() {
	suite.Result.Print()
	log.Println(">>>> Finished Prometheus EMF TestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var testRunners []*test_runner.TestRunner

func getTestRunners() []*test_runner.TestRunner {
	if testRunners == nil {
		testRunners = []*test_runner.TestRunner{
			{
				TestRunner: &UntypedTestRunner{},
			},
			{
				TestRunner: &TokenReplacementTestRunner{},
			},
			{
				TestRunner: &EMFFieldsTestRunner{},
			},
			{
				TestRunner: &RelabelTestRunner{},
			},
		}
	}
	return testRunners
}

func (suite *PrometheusEMFTestSuite) TestAllInSuite() {
	for _, testRunner := range getTestRunners() {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Prometheus EMF Test Suite Failed")
}

func (suite *PrometheusEMFTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestPrometheusEMFSuite(t *testing.T) {
	suite.Run(t, new(PrometheusEMFTestSuite))
}

