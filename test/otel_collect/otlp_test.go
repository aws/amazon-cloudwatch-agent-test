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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

const (
	otlpNamespace = "CWAgent"
	otlpRuntime   = 2 * time.Minute
	otlpEndpoint  = "http://127.0.0.1:4318"
)

type OtlpCollectTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*OtlpCollectTestRunner)(nil)

func (t *OtlpCollectTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Validate metrics
	for _, metricName := range t.GetMeasuredMetrics() {
		results = append(results, t.validateOtlpMetric(metricName))
	}

	// Validate traces
	results = append(results, t.validateTraces())

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

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.ExpectedDimensionValue{Value: aws.String(t.env.InstanceId)},
		},
	})
	if len(failed) > 0 {
		testResult.Reason = fmt.Errorf("failed to get dimensions for %s", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(otlpNamespace, metricName, dims, metric.AVERAGE, metric.MinuteStatPeriod)
	if err != nil {
		testResult.Reason = err
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		testResult.Reason = fmt.Errorf("metric %s values not valid", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *OtlpCollectTestRunner) validateTraces() status.TestResult {
	testResult := status.TestResult{
		Name:   "OTLP_Traces",
		Status: status.FAILED,
	}

	annotations := map[string]interface{}{
		"test_type":   "otel_collect_otlp",
		"instance_id": t.env.InstanceId,
	}

	err := base.ValidateTraceSegments(
		time.Now().Add(-otlpRuntime),
		time.Now(),
		annotations,
		nil,
	)
	if err != nil {
		testResult.Reason = err
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
	// Send test metrics via OTLP HTTP
	go t.sendTestMetrics()

	// Send test traces via OTLP
	go t.sendTestTraces()

	return nil
}

func (t *OtlpCollectTestRunner) sendTestMetrics() {
	// Send OTLP metrics via HTTP in a loop
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

func (t *OtlpCollectTestRunner) sendTestTraces() {
	generator := otlp.NewLoadGenerator(&base.TraceGeneratorConfig{
		Interval: 15 * time.Second,
		Annotations: map[string]interface{}{
			"test_type":   "otel_collect_otlp",
			"instance_id": t.env.InstanceId,
		},
		Attributes: []attribute.KeyValue{
			attribute.String("test_type", "otel_collect_otlp"),
			attribute.String("instance_id", t.env.InstanceId),
		},
	})
	generator.StartSendingTraces(context.Background()) //nolint:errcheck
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
	factory := dimension.GetDimensionFactory(*env)

	testRunner := &OtlpCollectTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{DimensionFactory: factory},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}
