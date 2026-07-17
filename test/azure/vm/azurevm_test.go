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
	"log"
	"net/http"
	"os"
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
	// agentLogFile lets us confirm the collector booted the Azure web-identity pipeline before asserting delivery.
	agentLogFile = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	// serviceName tags emitted telemetry so validation can isolate this test's records from other traffic.
	serviceName = "azurevm-otlp-test-service"
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
	go sendTelemetry(stop)
	time.Sleep(loadWindow)
	close(stop)
	// Allow final export + CloudWatch ingestion to settle before querying.
	time.Sleep(30 * time.Second)

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
		r := validateTraces()
		require.Equal(t, status.SUCCESSFUL, r.Status, "trace validation failed: %v", r.Reason)
	})
}

func measuredMetrics() []string { return []string{"azurevm_otlp_counter", "azurevm_otlp_gauge"} }

// validateLogs confirms the OTLP log record landed in the default:otel log group for this host.
func validateLogs() status.TestResult {
	testResult := status.TestResult{Name: "AzureVM_Logs", Status: status.FAILED}

	streams := awsservice.GetLogStreams(otlpLogGroup)
	if len(streams) == 0 {
		testResult.Reason = fmt.Errorf("no log streams found in %s", otlpLogGroup)
		return testResult
	}

	since := time.Now().Add(-loadWindow - time.Minute)
	until := time.Now()
	marker := fmt.Sprintf("azurevm_otlp_log_%s", env.InstanceId)
	for _, stream := range streams {
		err := awsservice.ValidateLogs(
			otlpLogGroup, *stream.LogStreamName, &since, &until,
			awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
		)
		if err == nil {
			testResult.Status = status.SUCCESSFUL
			return testResult
		}
		testResult.Reason = err
	}
	return testResult
}

// validateTraces confirms OTLP trace segments reached X-Ray by fetching them with BatchGetTraces
// using the exact trace IDs we generated. This bypasses GetTraceSummaries indexing (which can lag
// 5-10+ minutes for OTLP-ingested traces) and queries the raw trace store directly.
func validateTraces() status.TestResult {
	testResult := status.TestResult{Name: "AzureVM_Traces", Status: status.FAILED}

	if len(generatedTraceIDs) == 0 {
		testResult.Reason = fmt.Errorf("no trace IDs were generated during the load window")
		return testResult
	}

	xrayIDs := make([]string, len(generatedTraceIDs))
	for i, otlpID := range generatedTraceIDs {
		xrayIDs[i] = otlpToXRayTraceID(otlpID)
	}
	log.Printf("[AzureVM_Traces] generated %d trace IDs, querying X-Ray with BatchGetTraces (sample: %s)", len(xrayIDs), xrayIDs[0])

	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		traces, err := awsservice.GetBatchTraces(xrayIDs)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: BatchGetTraces error: %w", attempt, err)
		} else if len(traces) == 0 {
			testResult.Reason = fmt.Errorf("attempt %d: BatchGetTraces returned 0 traces for %d IDs (sample: %s)",
				attempt, len(xrayIDs), xrayIDs[0])
		} else {
			log.Printf("[AzureVM_Traces] attempt %d: BatchGetTraces returned %d/%d traces", attempt, len(traces), len(xrayIDs))
			testResult.Status = status.SUCCESSFUL
			return testResult
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
			post("/v1/traces", buildTracesPayload(env.InstanceId))
		}
	}
}

func post(path string, payload []byte) {
	req, err := http.NewRequest("POST", otlpEndpoint+path, bytes.NewReader(payload))
	if err != nil {
		log.Printf("failed to build OTLP request for %s: %v", path, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(req); err != nil {
		log.Printf("failed to POST OTLP to %s: %v", path, err)
	}
}
