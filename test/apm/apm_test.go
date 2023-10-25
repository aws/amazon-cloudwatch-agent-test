// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package apm

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

const (
	APMServerConsumerTestName = "APM-Server-Consumer"
	APMClientProducerTestName = "APM-Client-Producer"
	APMTracesTestName         = "APM-Traces"
)

type APMTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *APMTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting APMTestSuite")
}

func (suite *APMTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished APMTestSuite")
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
				Runner: &APMMetricsRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, APMServerConsumerTestName, "HostedIn.EKS.Cluster"},
				Env:    *env,
			},
			{
				Runner: &APMMetricsRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, APMClientProducerTestName, "HostedIn.EKS.Cluster"},
				Env:    *env,
			},
			{
				Runner: &APMTracesRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, APMTracesTestName, env.EKSClusterName},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *APMTestSuite) TestAllInSuite() {
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

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "APM Test Suite Failed")
}

func (suite *APMTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestAPMSuite(t *testing.T) {
	suite.Run(t, new(APMTestSuite))
}
