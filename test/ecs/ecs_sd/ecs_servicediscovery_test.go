// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ecs_sd

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

/*
Purpose:
1) Validate ECS ServiceDiscovery via multiple methods by publishing Prometheus EMF to CW  https://github.com/aws/amazon-cloudwatch-agent/blob/main/internal/ecsservicediscovery/README.md
   - docker_label
   - task_definition_list
   - service_name_list_for_tasks
2) Detect the changes in metadata endpoint for ECS Container Agent https://github.com/aws/amazon-cloudwatch-agent/blob/main/translator/util/ecsutil/ecsutil.go#L67-L75


Implementation:
1) Check if the LogGroupFormat correctly scrapes the clusterName from metadata endpoint (https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38)
2) Check if expected Prometheus EMF data is correctly published as logs and metrics to CloudWatch for each service discovery method
*/

var (
	ecsTestRunners []*test_runner.ECSTestRunner
)

func getEcsTestRunners(env *environment.MetaData) []*test_runner.ECSTestRunner {
	if len(ecsTestRunners) == 0 {

		ecsTestRunners = []*test_runner.ECSTestRunner{
			{
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "dockerLabel"},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
			{
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "taskDefinitionList"},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
			{
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "serviceNameList"},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
			{
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "combined"},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			},
			{
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "targetDeduplication"},
				RunStrategy: &test_runner.ECSAgentRunStrategy{},
				Env:         *env,
			}, {
				Runner:      &ECSServiceDiscoveryTestRunner{scenarioName: "targetDeduplication2"},
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
