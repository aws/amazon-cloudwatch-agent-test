// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"bytes"
	"context"
	"fmt"
	"log"
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
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	otlpRuntime  = 3 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"
	otlpGRPCAddr = "127.0.0.1:4317"
	traceTestType = "otel_collect_otlp_traces"
	otlpLogGroup  = "/aws/cwagent"
)

type OtlpCollectTestRunner struct {
	test_runner.BaseTestRunner
	env       *environment.MetaData
	startedAt time.Time
}

var _ test_runner.ITestRunner = (*OtlpCollectTestRunner)(nil)

func (t *OtlpCollectTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Metrics
	metricResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName(), t.env.Region, t.GetMeasuredMetrics(),
		otlpvalidation.OtlpMetricLabels(t.env.AgentStartCommand, t.env.InstanceId))
	results = append(results, metricResult.TestResults...)

	// Traces
	results = append(results, t.validateTraces())

	// Logs
	results = append(results, t.validateLogs())

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// validateTraces queries X-Ray for traces from this run, isolated by instance_id.
// startedAt is captured at SetupAfterAgentRun so the window is stable across retries.
func (t *OtlpCollectTestRunner) validateTraces() status.TestResult {
	annotations := map[string]interface{}{
		"test_type":   traceTestType,
		"instance_id": t.env.InstanceId,
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(30 * time.Second)
		}
		err = base.ValidateTraceSegments(t.startedAt, time.Now(), annotations, nil)
		if err == nil {
			return status.TestResult{Name: "OTLP_Traces", Status: status.SUCCESSFUL}
		}
	}
	return status.TestResult{Name: "OTLP_Traces", Status: status.FAILED, Reason: err}
}

// validateLogs checks that OTLP logs from this run are in CloudWatch Logs.
func (t *OtlpCollectTestRunner) validateLogs() status.TestResult {
	since := t.startedAt
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
	t.startedAt = time.Now()
	go func() {
		_ = common.SendOTLPMetrics(otlpEndpoint, t.env.InstanceId, 10*time.Second, otlpRuntime-30*time.Second)
	}()
	go t.generateTraces()
	go t.sendTestLogs()
	return nil
}

// generateTraces waits for gRPC port 4317, then streams traces to the agent.
// The wait is necessary because otlptracegrpc.New connects at construction time.
func (t *OtlpCollectTestRunner) generateTraces() {
	if err := common.WaitForTCPPort(otlpGRPCAddr, 2*time.Minute); err != nil {
		log.Printf("generateTraces: gRPC port not ready, skipping: %v", err)
		return
	}
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
			http.DefaultClient.Do(req) //nolint:errcheck
		}
	}
}

func buildOtlpLogsPayload(instanceId string) []byte {
	now := time.Now().UnixNano()
	// Routes to CW Logs via aws.log.group.name / aws.log.stream.name resource attributes.
	payload := fmt.Sprintf(`{
  "resourceLogs": [{
    "resource": {"attributes": [
      {"key": "aws.log.group.name", "value": {"stringValue": "%s"}},
      {"key": "aws.log.stream.name", "value": {"stringValue": "%s"}},
      {"key": "InstanceId", "value": {"stringValue": "%s"}}
    ]},
    "scopeLogs": [{
      "logRecords": [{
        "timeUnixNano": "%d",
        "severityText": "INFO",
        "body": {"stringValue": "otlp integration test log"},
        "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, otlpLogGroup, instanceId, instanceId, now, instanceId)
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
