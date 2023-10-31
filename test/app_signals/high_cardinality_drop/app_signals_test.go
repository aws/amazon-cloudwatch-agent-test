// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package app_signals

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

const (
	AppSignalsServerConsumerTestName = "AppSignals-Server-Consumer"
	AppSignalsClientProducerTestName = "AppSignals-Client-Producer"
	AppSignalsTracesTestName         = "AppSignals-Traces"
)

type AppSignalsTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *AppSignalsTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting AppSignalsTestSuite")
}

func (suite *AppSignalsTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished AppSignalsTestSuite")
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
				Runner: &AppSignalsMetricsRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, AppSignalsServerConsumerTestName, "HostedIn.EKS.Cluster"},
				Env:    *env,
			},
			{
				Runner: &AppSignalsMetricsRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, AppSignalsClientProducerTestName, "HostedIn.EKS.Cluster"},
				Env:    *env,
			},
			{
				Runner: &AppSignalsTracesRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, AppSignalsTracesTestName, env.EKSClusterName},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *AppSignalsTestSuite) TestAllInSuite() {
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

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "AppSignals Test Suite Failed")
}

func (suite *AppSignalsTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestAppSignalsSuite(t *testing.T) {
	suite.Run(t, new(AppSignalsTestSuite))
}
