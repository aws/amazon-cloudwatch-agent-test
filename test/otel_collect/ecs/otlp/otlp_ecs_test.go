// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"strings"
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

// ECSOtlpTestRunner validates that a workload (sidecar) running on the ECS daemon
// can publish OTLP metrics to the agent's OTLP receiver and have them reach CloudWatch.
type ECSOtlpTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*ECSOtlpTestRunner)(nil)

func (t *ECSOtlpTestRunner) GetTestName() string { return "ecs_otlp" }

// GetAgentConfigFileName returns "" — config is pre-loaded by Terraform via SSM, no restart needed.
func (t *ECSOtlpTestRunner) GetAgentConfigFileName() string { return "" }

func (t *ECSOtlpTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_test_counter", "otlp_test_gauge"}
}

func (t *ECSOtlpTestRunner) Validate() status.TestGroupResult {
	env := environment.GetEnvironmentMetaData()
	// Give the sidecar time to push and the agent to export before querying.
	time.Sleep(3 * time.Minute)
	// Isolate by cluster name — stamped by the agent's resourcedetection processor.
	labels := map[string]string{
		"@resource.aws.ecs.cluster.name": clusterNameFromArn(env.EcsClusterArn),
	}
	return otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName(), env.Region, t.GetMeasuredMetrics(), labels)
}

// clusterNameFromArn extracts the cluster name from the ECS cluster ARN.
func clusterNameFromArn(clusterArn string) string {
	if i := strings.LastIndex(clusterArn, "/"); i != -1 {
		return clusterArn[i+1:]
	}
	return clusterArn
}

func TestECSOtlpSuite(t *testing.T) {
	suite.Run(t, new(ECSOtlpTestSuite))
}

type ECSOtlpTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *ECSOtlpTestSuite) GetSuiteName() string {
	return "ECSOtlp"
}

func (suite *ECSOtlpTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	ecsTestRunner := &test_runner.ECSTestRunner{
		Runner:      &ECSOtlpTestRunner{},
		RunStrategy: &test_runner.ECSAgentRunStrategy{},
		Env:         *env,
	}
	ecsTestRunner.Run(suite, env)
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "ECS OTLP Test Suite Failed")
}
