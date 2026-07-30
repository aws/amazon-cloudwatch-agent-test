// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

// Package vm validates the agent on a real Azure VM running default:otel: it pushes OTLP to the
// pre-provisioned collector and verifies metrics/logs/traces reach CloudWatch via the Azure web-identity chain.
// Uses the TestMain/pre-provisioned pattern (not test_runner.TestRunner, which would restart the agent).
package vm

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// loadWindow is how long OTLP telemetry is pushed before validation; delivery + CloudWatch ingestion
	// need headroom beyond the push window.
	loadWindow   = 3 * time.Minute
	otlpEndpoint = "http://127.0.0.1:4318"
	// otlpLogGroup is where default:otel routes OTLP logs: "/aws/cwagent" + "/" + aws.log.source ("otlp").
	otlpLogGroup = "/aws/cwagent/otlp"
	// agentLogFile lets us confirm the collector booted the Azure web-identity pipeline before asserting delivery.
	agentLogFile = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	// serviceName tags emitted telemetry so validation can isolate this test's records from other traffic.
	serviceName = "azurevm-otlp-test-service"
	// spansLogGroup is where Transaction Search stores 100% of spans ingested via the X-Ray OTLP endpoint.
	spansLogGroup = "aws/spans"
)

var env *environment.MetaData

func TestMain(m *testing.M) {
	environment.RegisterEnvironmentMetaDataFlags()
	flag.Parse()
	env = environment.GetEnvironmentMetaData()
	if env.InstanceId == "" {
		fmt.Fprintln(os.Stderr, "instanceId flag is required (Azure VM ID / IMDS vmId) to scope telemetry")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// TestAzureVM confirms the pre-provisioned default:otel agent detected Azure, then pushes OTLP and validates
// that all three signals reach CloudWatch via the Azure web-identity chain.
func TestAzureVM(t *testing.T) {
	// The agent must already be running default:otel and have detected Azure before we generate load.
	agentLog := common.ReadAgentLogfile(agentLogFile)
	require.Contains(t, agentLog, "azure",
		"agent log has no \"azure\" marker; the default:otel Azure detection path was not exercised")

	// Push OTLP for the load window, then validate.
	stop := make(chan struct{})
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		sendTelemetry(stop)
	}()
	time.Sleep(loadWindow)
	close(stop)
	// Join the sender before reading what it recorded, rather than assuming the settle
	// sleep is long enough for its final iteration to finish.
	<-senderDone
	// Allow final export + CloudWatch ingestion to settle before querying.
	time.Sleep(30 * time.Second)

	// Snapshot the accepted trace IDs. The sender has exited, and the mutex still gives a
	// clean happens-before with its last append.
	traceMu.Lock()
	traceIDsCopy := make([]string, len(generatedTraceIDs))
	copy(traceIDsCopy, generatedTraceIDs)
	traceMu.Unlock()

	t.Run("Metrics", func(t *testing.T) {
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"AzureVMDefaultOtel", env.Region, measuredMetrics(),
			map[string]string{
				"@resource.host.id":        env.InstanceId,
				"@resource.cloud.provider": "azure",
			},
		)
		for _, r := range group.TestResults {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s: %v", r.Name, r.Reason)
		}
	})

	t.Run("Logs", func(t *testing.T) {
		require.Equal(t, status.SUCCESSFUL, validateLogs().Status)
	})

	t.Run("Traces", func(t *testing.T) {
		// Dump agent log errors/warnings from the load window to diagnose trace export issues.
		postLoadLog := common.ReadAgentLogfile(agentLogFile)
		for _, line := range filterLogLines(postLoadLog, "error", "warn", "xray", "traces", "401", "403", "500") {
			t.Logf("agent: %s", line)
		}
		r := validateTraces(traceIDsCopy)
		require.Equal(t, status.SUCCESSFUL, r.Status, "trace validation failed: %v", r.Reason)
	})
}

func measuredMetrics() []string { return []string{"azurevm_otlp_counter", "azurevm_otlp_gauge"} }

// validateLogs confirms the OTLP log record landed in the default:otel log group for this host.
func validateLogs() status.TestResult {
	testResult := status.TestResult{Name: "AzureVM_Logs", Status: status.FAILED}

	// Retry on the same schedule as the AKS log path: the stream and its events can both lag the
	// load window, so a single attempt fails on ingestion delay rather than on delivery.
	marker := fmt.Sprintf("azurevm_otlp_log_%s", env.InstanceId)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-loadWindow - time.Minute)
		until := time.Now()

		streams := awsservice.GetLogStreams(otlpLogGroup)
		if len(streams) == 0 {
			testResult.Reason = fmt.Errorf("attempt %d: no log streams found in %s", attempt, otlpLogGroup)
		}
		for _, stream := range streams {
			log.Printf("[AzureVM_Logs] attempt %d: checking %s/%s", attempt, otlpLogGroup, *stream.LogStreamName)
			err := awsservice.ValidateLogs(
				otlpLogGroup, *stream.LogStreamName, &since, &until,
				awsservice.AssertLogsNotEmpty(),
				awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
			)
			if err == nil {
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = err
		}
		if attempt < maxRetries {
			log.Printf("[AzureVM_Logs] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// validateTraces confirms every OTLP span emitted during the load window reached AWS through the
// X-Ray OTLP endpoint. That endpoint requires Transaction Search (trace segment destination =
// CloudWatchLogs), which stores 100% of ingested spans in the aws/spans log group; the X-Ray query
// APIs (GetTraceSummaries/BatchGetTraces) only see the indexed subset (1% by default), so aws/spans
// is the authoritative surface for OTLP trace delivery. Ingestion lags a few minutes, hence retries.
func validateTraces(traceIDs []string) status.TestResult {
	testResult := status.TestResult{Name: "AzureVM_Traces", Status: status.FAILED}

	if len(traceIDs) == 0 {
		testResult.Reason = fmt.Errorf("no trace IDs were generated during the load window")
		return testResult
	}

	quoted := make([]string, len(traceIDs))
	for i, id := range traceIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	query := fmt.Sprintf("fields traceId | filter traceId in [%s] | dedup traceId", strings.Join(quoted, ", "))
	log.Printf("[AzureVM_Traces] expecting %d trace IDs in %s (sample: %s)", len(traceIDs), spansLogGroup, traceIDs[0])

	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-loadWindow - 10*time.Minute)
		rows, err := awsservice.GetLogQueryResults(spansLogGroup, since.Unix(), time.Now().Unix(), query)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: %s query failed (is Transaction Search enabled in the account?): %w",
				attempt, spansLogGroup, err)
		} else {
			found := make(map[string]bool, len(rows))
			for _, row := range rows {
				for _, field := range row {
					if aws.ToString(field.Field) == "traceId" {
						found[aws.ToString(field.Value)] = true
					}
				}
			}
			var missing []string
			for _, id := range traceIDs {
				if !found[id] {
					missing = append(missing, id)
				}
			}
			if len(missing) == 0 {
				log.Printf("[AzureVM_Traces] attempt %d: all %d traces found in %s", attempt, len(traceIDs), spansLogGroup)
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: %d/%d traces missing from %s (first missing: %s)",
				attempt, len(missing), len(traceIDs), spansLogGroup, missing[0])
		}
		if attempt < maxRetries {
			log.Printf("[AzureVM_Traces] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// sendTelemetry pushes OTLP metrics, logs, and traces to the local collector until stop is closed.
func sendTelemetry(stop <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			post("/v1/metrics", buildMetricsPayload(env.InstanceId))
			post("/v1/logs", buildLogsPayload(env.InstanceId))
			// Only record the trace ID once the collector has accepted the span. Recording it
			// unconditionally would make a single transient POST failure guarantee a validation
			// failure for a trace that was never actually sent.
			payload, traceID := buildTracesPayload(env.InstanceId)
			if post("/v1/traces", payload) {
				recordTraceID(traceID)
			}
		}
	}
}

// post sends an OTLP payload and reports whether the collector accepted it.
func post(path string, payload []byte) bool {
	req, err := http.NewRequest("POST", otlpEndpoint+path, bytes.NewReader(payload))
	if err != nil {
		log.Printf("failed to build OTLP request for %s: %v", path, err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to POST OTLP to %s: %v", path, err)
		return false
	}
	// Drain before closing so the connection can be reused.
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Printf("OTLP POST to %s returned %s", path, resp.Status)
		return false
	}
	return true
}

// filterLogLines returns lines from a multi-line string that contain any of the given substrings (case-insensitive).
func filterLogLines(text string, substrs ...string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		for _, s := range substrs {
			if strings.Contains(lower, strings.ToLower(s)) {
				result = append(result, line)
				break
			}
		}
	}
	if len(result) > 50 {
		result = result[len(result)-50:]
	}
	return result
}
