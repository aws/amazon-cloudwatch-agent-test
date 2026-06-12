// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package host_insights

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const hostInsightsRuntime = 3 * time.Minute

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type HostInsightsTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*HostInsightsTestRunner)(nil)

func (t *HostInsightsTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics())
}

func (t *HostInsightsTestRunner) GetTestName() string                { return "HostInsights" }
func (t *HostInsightsTestRunner) GetAgentRunDuration() time.Duration { return hostInsightsRuntime }
func (t *HostInsightsTestRunner) GetAgentConfigFileName() string     { return "host_insights_config.json" }
func (t *HostInsightsTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"system.cpu.utilization",
		"system.memory.utilization",
		"system.filesystem.utilization",
		"system.network.io",
		"system.disk.operations",
	}
}

func TestHostInsights(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &HostInsightsTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
