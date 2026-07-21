// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ecs

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// ECSHostMetricsTestRunner validates that the OpenTelemetry (V2) host_metrics
// component, running as the CloudWatch Agent ECS daemon, publishes metrics to
// the OTLP endpoint (CloudWatch). The agent config is applied by terraform via
// the CW_CONFIG_CONTENT SSM parameter, so GetAgentConfigFileName returns "" and
// the ECS harness validates the already-running daemon without a restart.
type ECSHostMetricsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*ECSHostMetricsTestRunner)(nil)

func (t *ECSHostMetricsTestRunner) GetTestName() string { return "ecs_otlp_host_metrics" }

// GetAgentConfigFileName returns "" so the harness uses the config terraform
// already loaded from resources/config.json (no daemon restart needed).
func (t *ECSHostMetricsTestRunner) GetAgentConfigFileName() string { return "" }

func (t *ECSHostMetricsTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"system.cpu.utilization",
		"system.memory.utilization",
		"system.network.io",
		"system.disk.operations",
	}
}

func (t *ECSHostMetricsTestRunner) Validate() status.TestGroupResult {
	env := environment.GetEnvironmentMetaData()
	// Isolate this run's metrics by the ECS cluster ARN, which the agent's
	// resourcedetection processor stamps as a resource attribute on ECS.
	labels := map[string]string{
		"@resource.aws.ecs.cluster.arn": env.EcsClusterArn,
	}
	return otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName(), env.Region, t.GetMeasuredMetrics(), labels)
}

func TestECSOtelHostMetricsSuite(t *testing.T) {
	suite.Run(t, new(ECSHostMetricsTestSuite))
}

type ECSHostMetricsTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *ECSHostMetricsTestSuite) GetSuiteName() string {
	return "ECSOtelHostMetrics"
}

func (suite *ECSHostMetricsTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	ecsTestRunner := &test_runner.ECSTestRunner{
		Runner:      &ECSHostMetricsTestRunner{},
		RunStrategy: &test_runner.ECSAgentRunStrategy{},
		Env:         *env,
	}
	ecsTestRunner.Run(suite, env)
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "ECS OTLP Host Metrics Test Suite Failed")
}
