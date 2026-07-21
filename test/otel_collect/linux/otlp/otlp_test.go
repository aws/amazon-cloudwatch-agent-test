// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	otlpRuntime  = 3 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"

	// traceTestType is used as an X-Ray annotation value so the trace validator
	// can isolate this run's traces. It is set by the test (not derived from
	// IMDS), so it works in both EC2 and on-prem mode.
	traceTestType = "otel_collect_otlp_traces"

	// otlpLogGroup is the CloudWatch Logs group where OTLP logs published via the
	// V2 opentelemetry pipeline land. NOTE: confirm this matches the group the
	// agent actually writes to on a real run; adjust if the V2 OTLP logs path
	// uses a different group/stream.
	otlpLogGroup = "/aws/cwagent"
)

type OtlpCollectTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*OtlpCollectTestRunner)(nil)

func (t *OtlpCollectTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Metrics
	metricResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics(),
		otlpvalidation.ResourceHostIDLabels(t.env.AgentStartCommand, t.env.InstanceId))
	results = append(results, metricResult.TestResults...)

	// Traces
	results = append(results, t.validateTraces())

	// Logs
	results = append(results, t.validateLogs())

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// validateTraces confirms the OTLP traces this run generated are queryable in
// X-Ray, filtered by the instance_id annotation for isolation.
func (t *OtlpCollectTestRunner) validateTraces() status.TestResult {
	annotations := map[string]interface{}{
		"test_type":   traceTestType,
		"instance_id": t.env.InstanceId,
	}
	err := base.ValidateTraceSegments(time.Now().Add(-otlpRuntime), time.Now(), annotations, nil)
	if err != nil {
		return status.TestResult{Name: "OTLP_Traces", Status: status.FAILED, Reason: err}
	}
	return status.TestResult{Name: "OTLP_Traces", Status: status.SUCCESSFUL}
}

// validateLogs confirms the OTLP logs this run published are present in
// CloudWatch Logs for this instance.
func (t *OtlpCollectTestRunner) validateLogs() status.TestResult {
	since := time.Now().Add(-otlpRuntime)
	until := time.Now()

	if len(awsservice.GetLogStreams(otlpLogGroup)) == 0 {
		return status.TestResult{Name: "OTLP_Logs", Status: status.FAILED,
			Reason: fmt.Errorf("no log streams found in log group %s", otlpLogGroup)}
	}

	err := awsservice.ValidateLogs(
		otlpLogGroup,
		t.env.InstanceId,
		&since,
		&until,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"InstanceId\":\"%s\"", t.env.InstanceId)),
		),
	)
	if err != nil {
		return status.TestResult{Name: "OTLP_Logs", Status: status.FAILED, Reason: err}
	}
	return status.TestResult{Name: "OTLP_Logs", Status: status.SUCCESSFUL}
}

func (t *OtlpCollectTestRunner) GetTestName() string                { return "OtelCollectOTLP" }
func (t *OtlpCollectTestRunner) GetAgentRunDuration() time.Duration { return otlpRuntime }
func (t *OtlpCollectTestRunner) GetAgentConfigFileName() string     { return "otlp_config.json" }
func (t *OtlpCollectTestRunner) GetMeasuredMetrics() []string {
	return []string{"otlp_test_counter", "otlp_test_gauge"}
}

func (t *OtlpCollectTestRunner) SetupAfterAgentRun() error {
	go t.sendTestMetrics()
	go t.generateTraces()
	go t.sendTestLogs()
	return nil
}

// generateTraces pushes OTLP traces to the agent's OTLP gRPC receiver
func (t *OtlpCollectTestRunner) generateTraces() {
	generator := otlp.NewLoadGenerator(&base.TraceGeneratorConfig{
		Interval: 10 * time.Second,
		Annotations: map[string]interface{}{
			"test_type":   traceTestType,
			"instance_id": t.env.InstanceId,
		},
		Attributes: []attribute.KeyValue{
			attribute.String("test_type", traceTestType),
			attribute.String("instance_id", t.env.InstanceId),
		},
	})
	generator.StartSendingTraces(context.Background())
}

// sendTestLogs pushes OTLP logs to the agent's OTLP HTTP receiver (/v1/logs).
func (t *OtlpCollectTestRunner) sendTestLogs() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(otlpRuntime - 30*time.Second)
	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			payload := buildOtlpLogsPayload(t.env.InstanceId)
			req, _ := http.NewRequest("POST", otlpEndpoint+"/v1/logs", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			http.DefaultClient.Do(req)
		}
	}
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

var startTime = time.Now().UnixNano()

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
            "dataPoints": [{"asInt": "1", "startTimeUnixNano": "%d", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}],
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
}`, instanceId, startTime, now, instanceId, now, instanceId)
	return []byte(payload)
}

func buildOtlpLogsPayload(instanceId string) []byte {
	now := time.Now().UnixNano()
	payload := fmt.Sprintf(`{
  "resourceLogs": [{
    "resource": {"attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]},
    "scopeLogs": [{
      "logRecords": [{
        "timeUnixNano": "%d",
        "severityText": "INFO",
        "body": {"stringValue": "otlp integration test log"},
        "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, instanceId, now, instanceId)
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
