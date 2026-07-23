// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ecs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// ECSHostMetricsTestRunner validates that the V2 host_metrics component, running
// as an ECS daemon, publishes metrics to CloudWatch via the OTLP endpoint.
type ECSHostMetricsTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*ECSHostMetricsTestRunner)(nil)

func (t *ECSHostMetricsTestRunner) GetTestName() string { return "ecs_otlp_host_metrics" }

// GetAgentConfigFileName returns "" — config is pre-loaded by Terraform via SSM, no restart needed.
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
	time.Sleep(3 * time.Minute)
	// Filter by ECS cluster ARN stamped by the resourcedetection processor.
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
