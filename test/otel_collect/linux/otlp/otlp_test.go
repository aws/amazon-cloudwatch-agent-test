// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlp

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	otlpRuntime  = 3 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"
	otlpLogGroup = "/aws/cwagent"
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

	// Logs
	results = append(results, t.validateLogs())

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
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
	go t.sendTestLogs()
	return nil
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
			req, _ := http.NewRequest("POST", otlpEndpoint+"/v1/logs", bytes.NewReader(payload)) //nolint:errcheck 
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
