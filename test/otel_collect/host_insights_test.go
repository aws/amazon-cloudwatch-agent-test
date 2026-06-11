// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const hostInsightsRuntime = 2 * time.Minute

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type HostInsightsTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*HostInsightsTestRunner)(nil)

func (t *HostInsightsTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateHostMetric(metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *HostInsightsTestRunner) validateHostMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	client, err := otelmetrics.NewClient(context.Background(), otelmetrics.TestConfig{
		Region:         getRegion(t.env),
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", getRegion(t.env)),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		SigningService: "monitoring",
	})
	if err != nil {
		testResult.Reason = fmt.Errorf("creating otel metrics client: %w", err)
		return testResult
	}

	query := fmt.Sprintf(`{__name__="%s","@resource.host.id"="%s"}`, metricName, t.env.InstanceId)
	results, err := client.Query(context.Background(), query)
	if err != nil {
		testResult.Reason = fmt.Errorf("querying %s: %w", metricName, err)
		return testResult
	}

	if len(results) == 0 {
		testResult.Reason = fmt.Errorf("metric %s not found", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
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
