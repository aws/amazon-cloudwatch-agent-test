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
	// serviceName tags emitted telemetry so validation can isolate this test's records from other traffic.
	serviceName = "azurevm-otlp-test-service"
	// spanName is the emitted span's name, asserted verbatim in trace-content validation.
	spanName = "azurevm-otlp-test-span"
	// spansLogGroup is where Transaction Search stores 100% of spans ingested via the X-Ray OTLP endpoint.
	spansLogGroup = "aws/spans"
)

var env *environment.MetaData

// azureResourceExpectations is the shared source of truth for the Azure resourcedetection attributes
// asserted on every signal: an exact value, or PresenceOnly for present-and-non-empty. cloud.account.id
// and cloud.resource_id stay presence-only so the subscription id they carry never reaches the CI logs.
func azureResourceExpectations() map[string]string {
	return map[string]string{
		"cloud.provider":              "azure",
		"cloud.platform":              "azure.vm",
		"cloud.account.id":            otlpvalidation.PresenceOnly,
		"cloud.resource_id":           otlpvalidation.PresenceOnly,
		"cloud.region":                env.AzureLocation,
		"azure.vm.name":               env.AzureVMName,
		"azure.vm.size":               env.AzureVMSize,
		"azure.resourcegroup.name":    env.AzureResourceGroup,
		"deployment.environment.name": "azure.vm:" + env.AzureResourceGroup,
		// Presence only: this test's payloads set it, but the run's host metrics carry an unknown_service* value.
		"service.name": otlpvalidation.PresenceOnly,
		// host.name is the OS hostname, which Windows truncates to 15 chars, so it cannot equal the full
		// Azure VM name. VM identity is asserted exactly via azure.vm.name (IMDS) and host.id (vmId) instead.
		"host.name": otlpvalidation.PresenceOnly,
		"host.id":   env.InstanceId,
	}
}

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
	// The agent must already be running default:otel with the Azure VM resource detected. The
	// resourcedetection processor logs the detected resource, cloud.platform included, to the agent log.
	agentLog := common.ReadAgentLogfile(common.AgentLogFile)
	require.Contains(t, agentLog, `"cloud.platform":"azure.vm"`,
		"agent log has no azure.vm resource detection, so the default:otel Azure VM path was not exercised")

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
		// The payload carries only service.name + host.id, so every cloud.*/azure.* label here proves the
		// agent's resourcedetection enrichment.
		labels := map[string]string{}
		for attr, want := range azureResourceExpectations() {
			labels["@resource."+attr] = otlpvalidation.ExpectedValue(want)
		}
		for attr, want := range otlpvalidation.ScopeExpectations() {
			labels["@instrumentation."+attr] = otlpvalidation.ExpectedValue(want)
		}
		group := otlpvalidation.ValidateOtlpMetricsWithLabels(
			"AzureVMDefaultOtel", env.Region, measuredMetrics(), labels)
		for _, r := range group.TestResults {
			require.Equal(t, status.SUCCESSFUL, r.Status, "metric %s: %v", r.Name, r.Reason)
		}
	})

	t.Run("Logs", func(t *testing.T) {
		require.Equal(t, status.SUCCESSFUL, validateLogs().Status)
	})

	t.Run("Traces", func(t *testing.T) {
		// Dump agent log errors/warnings from the load window to diagnose trace export issues.
		postLoadLog := common.ReadAgentLogfile(common.AgentLogFile)
		for _, line := range filterLogLines(postLoadLog, "error", "warn", "xray", "traces", "401", "403", "500") {
			t.Logf("agent: %s", line)
		}
		r := validateTraces(traceIDsCopy)
		require.Equal(t, status.SUCCESSFUL, r.Status, "trace validation failed: %v", r.Reason)
	})
}

func measuredMetrics() []string {
	return []string{
		// Synthetic OTLP metrics this test pushes.
		"azurevm_otlp_counter",
		"azurevm_otlp_gauge",
		// Host metrics from default:otel's host_metrics.
		"system.cpu.utilization",
		"system.memory.utilization",
		"system.filesystem.utilization",
	}
}

// validateLogs confirms the OTLP log record landed in the default:otel log group on the stream the
// agent's log routing is expected to derive for this host.
func validateLogs() status.TestResult {
	testResult := status.TestResult{Name: "AzureVM_Logs", Status: status.FAILED}

	// The agent routes OTLP logs to {host.id}/{service.name}, so assert that exact stream: it makes the
	// check prove log routing rather than just delivery, and keeps cost flat as the shared group
	// accumulates a stream per VM. Retries match the AKS path, since the stream and events both lag.
	logStream := fmt.Sprintf("%s/%s", env.InstanceId, serviceName)
	// Clean up only on success: the group is shared by every VM run, so drop this run's stream but never
	// the group. On failure the stream is left in place as evidence for whoever debugs the run.
	defer func() {
		if testResult.Status == status.SUCCESSFUL {
			awsservice.DeleteLogStream(otlpLogGroup, logStream)
		}
	}()
	// The stored log event is the full OTLP record as JSON, so parse and assert its content.
	marker := fmt.Sprintf("azurevm_otlp_log_%s", env.InstanceId)
	const maxRetries = 4
	const retryInterval = 30 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		since := time.Now().Add(-loadWindow - time.Minute)
		until := time.Now()
		log.Printf("[AzureVM_Logs] attempt %d: checking %s/%s", attempt, otlpLogGroup, logStream)
		err := awsservice.ValidateLogs(
			otlpLogGroup, logStream, &since, &until,
			awsservice.AssertLogsNotEmpty(),
			awsservice.AssertPerLog(otlpvalidation.AssertLogRecord(func(rec otlpvalidation.LogRecord) error {
				return otlpvalidation.AssertLogContent(rec, marker, "INFO", azureResourceExpectations())
			})),
		)
		if err == nil {
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		testResult.Reason = err
		if attempt < maxRetries {
			log.Printf("[AzureVM_Logs] %v — retrying in %v", testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}

// validateTraces confirms every emitted span reached Transaction Search (aws/spans) with the expected
// content. The generic query/retry/parse loop lives in otlpvalidation.ValidateOtlpTraces.
func validateTraces(traceIDs []string) status.TestResult {
	// The payload sends kind 2, which Transaction Search records as "SERVER".
	return otlpvalidation.ValidateOtlpTraces("AzureVM_Traces", spansLogGroup, traceIDs, func(s otlpvalidation.SpanRecord) error {
		return otlpvalidation.AssertSpanContent(s, spanName, "SERVER",
			map[string]string{"instance_id": env.InstanceId}, azureResourceExpectations())
	})
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
