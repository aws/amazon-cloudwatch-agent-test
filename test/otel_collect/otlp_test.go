// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_collect

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	otlpRuntime  = 2 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"
)

type OtlpCollectTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*OtlpCollectTestRunner)(nil)

func (t *OtlpCollectTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	for _, metricName := range t.GetMeasuredMetrics() {
		results = append(results, t.validateOtlpMetric(metricName))
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *OtlpCollectTestRunner) validateOtlpMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	client, err := otelmetrics.NewClient(context.Background(), otelmetrics.TestConfig{
		Region:         t.env.Region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", t.env.Region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		SigningService: "monitoring",
	})
	if err != nil {
		testResult.Reason = fmt.Errorf("creating otel metrics client: %w", err)
		return testResult
	}

	query := fmt.Sprintf(`{__name__="%s","InstanceId"="%s"}`, metricName, t.env.InstanceId)
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

func (t *OtlpCollectTestRunner) GetTestName() string                { return "OtelCollectOTLP" }
func (t *OtlpCollectTestRunner) GetAgentRunDuration() time.Duration { return otlpRuntime }
func (t *OtlpCollectTestRunner) GetAgentConfigFileName() string     { return "otlp_config.json" }
func (t *OtlpCollectTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_test_counter", "otlp_test_gauge"}
}

func (t *OtlpCollectTestRunner) SetupAfterAgentRun() error {
	go t.sendTestMetrics()
	return nil
}

func (t *OtlpCollectTestRunner) sendTestMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(otlpRuntime - 30*time.Second)
	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			payload := buildOtlpMetricsPayload(t.env.InstanceId)
			req, _ := http.NewRequest("POST", otlpEndpoint+"/v1/metrics", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			http.DefaultClient.Do(req) //nolint:errcheck
		}
	}
}

func buildOtlpMetricsPayload(instanceId string) []byte {
	now := time.Now().UnixNano()
	payload := fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]},
    "scopeMetrics": [{
      "metrics": [
        {
          "name": "otlp_test_counter",
          "sum": {
            "dataPoints": [{"asInt": "1", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}],
            "isMonotonic": true,
            "aggregationTemporality": 2
          }
        },
        {
          "name": "otlp_test_gauge",
          "gauge": {
            "dataPoints": [{"asDouble": 42.0, "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        }
      ]
    }]
  }]
}`, instanceId, now, instanceId, now, instanceId)
	return []byte(payload)
}

func TestOTLPCollect(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &OtlpCollectTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}
