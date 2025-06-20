// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package ecs_sd

import (
	_ "embed"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/stretchr/testify/suite"
	"testing"
)

/*
Purpose:
1) Validate ECS ServiceDiscovery via DockerLabels by publishing Prometheus EMF to CW  https://github.com/aws/amazon-cloudwatch-agent/blob/main/internal/ecsservicediscovery/README.md
2) Detect the changes in metadata endpoint for ECS Container Agent https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/util/ecsutil/ecsutil.go#L67-L75


Implementation:
1) Check if the LogGroupFormat correctly scrapes the clusterName from metadata endpoint (https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38)
2) Check if expected Prometheus EMF data is correctly published as logs and metrics to CloudWatch
*/

var (
	ecsTestRunners []*test_runner.ECSTestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*test_runner.ECSTestRunner {
	if len(ecsTestRunners) == 0 {

		ecsTestRunners = []*test_runner.ECSTestRunner{
			{
				Runner:      &ECSServiceDiscoveryTestRunner{},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
		}
	}
	return ecsTestRunners
}

var _ test_runner.ITestRunner = (*ECSServiceDiscoveryTestRunner)(nil)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestECSServiceDiscoveryTestSuite(t *testing.T) {
	suite.Run(t, new(ECSServiceDiscoveryTestSuite))
}

type ECSServiceDiscoveryTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *ECSServiceDiscoveryTestSuite) GetSuiteName() string {
	return "ECSServiceDiscovery"
}

func (suite *ECSServiceDiscoveryTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	for _, ecsTestRunner := range getEcsTestRunners(env) {
		ecsTestRunner.Run(suite, env)
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "ECS ServiceDiscovery Test Suite Failed")
}
