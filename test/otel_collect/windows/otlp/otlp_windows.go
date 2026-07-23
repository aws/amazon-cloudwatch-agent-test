// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package otlp

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	otlptraces "github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/otlp"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

//go:embed resources/config.json
var testConfigJSON string

const (
	tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"
	otlpRuntime   = 3 * time.Minute
	sendInterval  = 10 * time.Second
	otlpEndpoint  = "http://127.0.0.1:4318"
	otlpGRPCAddr  = "127.0.0.1:4317"
	traceTestType = "otel_collect_windows_otlp_traces"
	otlpLogGroup  = "/aws/cwagent"
	// spansLogGroup is where V2 OTLP traces land (CloudWatch Logs destination).
	spansLogGroup = "aws/spans"
)

func Validate() error {
	env := environment.GetEnvironmentMetaData()

	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := common.CopyFile(tmpConfigPath, common.ConfigOutputPath); err != nil {
		return fmt.Errorf("could not copy config: %w", err)
	}
	if err := common.StartAgent(common.ConfigOutputPath, true, false); err != nil {
		return fmt.Errorf("could not start agent: %w", err)
	}
	// Wait for HTTP receiver before sending data.
	if err := common.WaitForOTLPEndpoint(otlpEndpoint, 2*time.Minute); err != nil {
		return fmt.Errorf("OTLP endpoint not ready: %w", err)
	}

	// Send metrics, logs, and traces concurrently.
	startedAt := time.Now()
	go func() {
		_ = common.SendOTLPMetrics(otlpEndpoint, env.InstanceId, sendInterval, otlpRuntime)
	}()
	go sendLogs(env.InstanceId)
	go generateTraces(env.InstanceId)

	time.Sleep(otlpRuntime)
	_ = common.StopAgent()

	// Validate metrics.
	if err := otelmetrics.AssertMetricsPresent(
		context.Background(),
		env.Region,
		[]string{"otlp_test_counter", "otlp_test_gauge"},
		otlpvalidation.OtlpMetricLabels(env.AgentStartCommand, env.InstanceId),
		3,
		30*time.Second,
	); err != nil {
		return fmt.Errorf("metrics validation failed: %w", err)
	}

	// Validate traces.
	if err := validateTraces(startedAt, env.InstanceId); err != nil {
		return fmt.Errorf("traces validation failed: %w", err)
	}

	// Validate logs.
	if err := validateLogs(startedAt, env.InstanceId); err != nil {
		return fmt.Errorf("logs validation failed: %w", err)
	}

	return nil
}

func generateTraces(instanceID string) {
	if err := common.WaitForTCPPort(otlpGRPCAddr, 2*time.Minute); err != nil {
		log.Printf("generateTraces: gRPC port not ready, skipping: %v", err)
		return
	}
	generator := otlptraces.NewLoadGenerator(&base.TraceGeneratorConfig{
		Interval: 10 * time.Second,
		Annotations: map[string]interface{}{
			"test_type":   traceTestType,
			"instance_id": instanceID,
		},
		Attributes: []attribute.KeyValue{
			attribute.String("test_type", traceTestType),
			attribute.String("instance_id", instanceID),
		},
	})
	generator.StartSendingTraces(context.Background())
}

func validateTraces(startedAt time.Time, instanceID string) error {
	query := fmt.Sprintf(`fields @message | filter @message like /%s/ | limit 100`, instanceID)
	start := startedAt.Unix()
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(30 * time.Second)
		}
		results, qErr := awsservice.GetLogQueryResults(spansLogGroup, start, time.Now().Unix(), query)
		if qErr != nil {
			err = qErr
			continue
		}
		log.Printf("[OTLP_Traces] attempt %d: found %d spans in %s for instance %s", attempt+1, len(results), spansLogGroup, instanceID)
		if len(results) > 0 {
			return nil
		}
		err = fmt.Errorf("no spans found in %s for instance %s", spansLogGroup, instanceID)
	}
	return err
}

func sendLogs(instanceID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeout := time.After(otlpRuntime - 30*time.Second)
	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			payload := buildLogsPayload(instanceID)
			req, _ := http.NewRequest("POST", otlpEndpoint+"/v1/logs", bytes.NewReader(payload)) //nolint:errcheck
			req.Header.Set("Content-Type", "application/json")
			http.DefaultClient.Do(req) //nolint:errcheck
		}
	}
}

func validateLogs(startedAt time.Time, instanceID string) error {
	since := startedAt
	until := time.Now()
	if len(awsservice.GetLogStreams(otlpLogGroup)) == 0 {
		return fmt.Errorf("no log streams found in %s", otlpLogGroup)
	}
	return awsservice.ValidateLogs(
		otlpLogGroup,
		instanceID,
		&since,
		&until,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"InstanceId\":\"%s\"", instanceID)),
		),
	)
}

func buildLogsPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	return []byte(fmt.Sprintf(`{
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
        "body": {"stringValue": "otlp windows integration test log"},
        "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, otlpLogGroup, instanceID, instanceID, now, instanceID))
}
