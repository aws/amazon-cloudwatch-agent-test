// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package host_metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const hostMetricsRuntime = 3 * time.Minute

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type HostMetricsTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*HostMetricsTestRunner)(nil)

func (t *HostMetricsTestRunner) Validate() status.TestGroupResult {
	return otlpvalidation.ValidateOtlpMetrics(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics())
}

func (t *HostMetricsTestRunner) GetTestName() string                { return "HostMetrics" }
func (t *HostMetricsTestRunner) GetAgentRunDuration() time.Duration { return hostMetricsRuntime }
func (t *HostMetricsTestRunner) GetAgentConfigFileName() string     { return "host_metrics_config.json" }
func (t *HostMetricsTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"system.cpu.utilization",
		"system.memory.utilization",
		"system.filesystem.utilization",
		"system.network.io",
		"system.disk.operations",
	}
}

func TestHostMetrics(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &HostMetricsTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s failed: %v", r.Name, r.Reason)
	}
}
