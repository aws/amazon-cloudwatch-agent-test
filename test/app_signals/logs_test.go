// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Tests for the Application Signals OTLP logs pipeline: dynamic per-service
// log group/stream routing via the CW OTLP endpoint.
//
// Uses CWAgent JSON config → translator → OTel YAML (the standard customer path).
//
// Test cases:
//   - TestAppSignalsLogsDynamicRouting: Multiple services route to separate log groups
//   - TestAppSignalsLogsNoisyNeighbor: Service B's creation fails, Service A is not blocked
//   - TestAppSignalsLogsDefaultPlaceholder: Missing attrs default to unknown_service/unknown
//   - TestAppSignalsLogsRouting: Batch vs no-batch pipeline routing
//   - TestAppSignalsMetricsRouting: EMF vs OTLP metric routing
package app_signals

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	logsConfigPath = "agent_configs/config_logs_dynamic_placeholders.json"
	sleepForFlush  = 60 * time.Second
	logGroupPrefix = "/test/otlp-dynamic/"

	// Resource attribute values used in test logs.
	// log_stream_name template is "{host.name}/{service.instance.id}"
	testHostName   = "test-host"
	testInstanceID = "instance-001"
	testStreamName = testHostName + "/" + testInstanceID
)

func startLogsAgent(t *testing.T) {
	t.Helper()
	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	common.CopyFile(logsConfigPath, common.ConfigOutputPath)
	err := common.StartAgent(common.ConfigOutputPath, true, false)
	require.NoError(t, err, "Failed to start agent")
}

// TestAppSignalsLogsDynamicRouting verifies that logs from different services are
// routed to separate, dynamically-created log groups.
func TestAppSignalsLogsDynamicRouting(t *testing.T) {
	services := []string{"service-a", "service-b", "service-c"}
	numLogsPerService := 5

	for _, svc := range services {
		defer awsservice.DeleteLogGroupAndStream(logGroupPrefix+svc, testStreamName)
	}
	// "unknown_service:java" should be truncated to "unknown_service" by OTTL
	defer awsservice.DeleteLogGroupAndStream(logGroupPrefix+"unknown_service", testStreamName)

	startLogsAgent(t)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	for _, svc := range services {
		sendOTLPLogs(t, svc, numLogsPerService)
	}
	sendOTLPLogs(t, "unknown_service:java", numLogsPerService)

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	agentLog, _ := os.ReadFile(common.AgentLogFile)
	t.Logf("Agent logs (tail):\n%s", tail(string(agentLog), 2000))

	for _, svc := range services {
		logGroup := logGroupPrefix + svc
		t.Run(svc, func(t *testing.T) {
			// Verify log stream name resolved from placeholders
			streams := awsservice.GetLogStreams(logGroup)
			require.NotEmpty(t, streams, "No log streams found in %s", logGroup)
			assert.Equal(t, testStreamName, *streams[0].LogStreamName,
				"Log stream name should be resolved from {host.name}/{service.instance.id} placeholders")

			// Exact count + content match proves no cross-contamination
			events, err := awsservice.GetLogsSince(logGroup, testStreamName, &start, &end)
			require.NoError(t, err, "Failed to get logs for service %s in log group %s", svc, logGroup)
			assert.Equal(t, numLogsPerService, len(events),
				"Expected exactly %d logs in %s", numLogsPerService, logGroup)
			for _, event := range events {
				assert.Contains(t, *event.Message, svc,
					"All logs in %s should belong to service %s", logGroup, svc)
			}
		})
	}

	// Verify "unknown_service:java" was truncated to "unknown_service"
	t.Run("unknown_service_truncation", func(t *testing.T) {
		logGroup := logGroupPrefix + "unknown_service"
		err := awsservice.ValidateLogs(
			logGroup,
			testStreamName,
			&start,
			&end,
			awsservice.AssertLogsCount(numLogsPerService),
			awsservice.AssertLogsNotEmpty(),
		)
		assert.NoError(t, err,
			"Logs with service.name='unknown_service:java' should be truncated to 'unknown_service'")
	})

	// Verify temporary/internal attributes are cleaned up and not present in exported logs
	t.Run("internal_attributes_cleaned_up", func(t *testing.T) {
		logGroup := logGroupPrefix + services[0]
		events, err := awsservice.GetLogsSince(logGroup, testStreamName, &start, &end)
		require.NoError(t, err)
		require.NotEmpty(t, events)

		internalAttrs := []string{
			"temporary_key.",
			"aws.log.group.name",
			"aws.log.stream.name",
		}
		for _, event := range events {
			for _, attr := range internalAttrs {
				assert.NotContains(t, *event.Message, attr,
					"Exported log should not contain internal attribute %q", attr)
			}
		}
	})
}

// TestAppSignalsLogsNoisyNeighbor verifies that when one service's log group cannot be created
// (e.g., due to an invalid name), logs from other services are not blocked.
func TestAppSignalsLogsNoisyNeighbor(t *testing.T) {
	goodService := "healthy-service"
	badService := "bad:service:name"

	goodLogGroup := logGroupPrefix + goodService
	badLogGroup := logGroupPrefix + badService

	defer awsservice.DeleteLogGroupAndStream(goodLogGroup, testStreamName)
	defer awsservice.DeleteLogGroupAndStream(badLogGroup, testStreamName)

	numLogs := 10

	startLogsAgent(t)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	for i := 0; i < numLogs; i++ {
		sendOTLPLogs(t, goodService, 1)
		sendOTLPLogs(t, badService, 1)
	}

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	agentLog, _ := os.ReadFile(common.AgentLogFile)
	t.Logf("Agent logs (tail):\n%s", tail(string(agentLog), 2000))

	t.Run("healthy_service_not_blocked", func(t *testing.T) {
		err := awsservice.ValidateLogs(
			goodLogGroup,
			testStreamName,
			&start,
			&end,
			awsservice.AssertLogsCount(numLogs),
		)
		assert.NoError(t, err,
			"Healthy service logs were blocked by noisy neighbor! "+
				"Expected %d logs in %s but validation failed", numLogs, goodLogGroup)
	})

	t.Run("bad_service_loggroup_not_created", func(t *testing.T) {
		exists := awsservice.IsLogGroupExists(badLogGroup)
		assert.False(t, exists,
			"Bad service log group %s should not exist", badLogGroup)
	})

	t.Run("agent_logs_show_creation_failure", func(t *testing.T) {
		agentLogStr := string(agentLog)
		if !strings.Contains(agentLogStr, "caller") {
			t.Skip("Agent log file does not contain OTel-level logs")
		}
		assert.Contains(t, agentLogStr, "Failed to create log group",
			"Agent logs should contain creation failure message for bad service")
	})
}

// TestAppSignalsLogsDefaultPlaceholder verifies that when service.name is missing,
// the translator's OTTL fallback defaults service.name to "unknown_service" and
// other missing attributes to "unknown", so logs route to
// "/test/otlp-dynamic/unknown_service" with stream "unknown/unknown".
func TestAppSignalsLogsDefaultPlaceholder(t *testing.T) {
	fallbackLogGroup := logGroupPrefix + "unknown_service"
	fallbackStreamName := "unknown/unknown"
	numLogs := 3

	defer awsservice.DeleteLogGroupAndStream(fallbackLogGroup, fallbackStreamName)

	startLogsAgent(t)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	sendOTLPLogsNoService(t, numLogs)

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	err := awsservice.ValidateLogs(
		fallbackLogGroup,
		fallbackStreamName,
		&start,
		&end,
		awsservice.AssertLogsCount(numLogs),
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err,
		"Logs with missing service.name should land in %s with stream %s",
		fallbackLogGroup, fallbackStreamName)

	// Verify no failed log group creation — unhandled nil would produce an invalid name
	// that the provisioner would fail to create
	agentLog, _ := os.ReadFile(common.AgentLogFile)
	assert.NotContains(t, string(agentLog), "Failed to create log group/stream",
		"Agent should not attempt to create a log group with an invalid name from unhandled nil")
}

// TestAppSignalsLogsRouting verifies that the routing connector correctly
// splits logs between the batch and no-batch pipelines:
//   - 4 regular logs → batch pipeline (same ingestion time)
//   - 1 aggregate_profile log → no-batch pipeline (different ingestion time)
func TestAppSignalsLogsRouting(t *testing.T) {
	serviceName := "routing-test-svc"
	logGroup := logGroupPrefix + serviceName

	defer awsservice.DeleteLogGroupAndStream(logGroup, testStreamName)

	startLogsAgent(t)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	// Send logs: regular, regular, aggregate_profile, regular, regular
	sendOTLPLogs(t, serviceName, 2)
	sendAggregateProfileLogs(t, serviceName, 1)
	sendOTLPLogs(t, serviceName, 2)

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	// Validate all 5 logs arrived
	events, err := awsservice.GetLogsSince(logGroup, testStreamName, &start, &end)
	require.NoError(t, err, "Failed to get logs from %s", logGroup)
	require.Equal(t, 5, len(events), "Expected 5 total logs (4 batched + 1 unbatched)")

	// Separate batched vs unbatched by message content
	var batchedIngestionTimes []int64
	var unbatchedIngestionTimes []int64
	for _, event := range events {
		if event.IngestionTime == nil {
			continue
		}
		if strings.Contains(*event.Message, "Aggregate profile") {
			unbatchedIngestionTimes = append(unbatchedIngestionTimes, *event.IngestionTime)
		} else {
			batchedIngestionTimes = append(batchedIngestionTimes, *event.IngestionTime)
		}
	}

	require.Equal(t, 4, len(batchedIngestionTimes), "Expected 4 batched logs")
	require.Equal(t, 1, len(unbatchedIngestionTimes), "Expected 1 unbatched log")

	// All 4 batched logs should share the same ingestion time (flushed together)
	for _, bt := range batchedIngestionTimes {
		assert.Equal(t, batchedIngestionTimes[0], bt,
			"All batched logs should have identical ingestion times")
	}

	// Unbatched log should have a different ingestion time than batched logs
	assert.NotEqual(t, unbatchedIngestionTimes[0], batchedIngestionTimes[0],
		"Unbatched log (aggregate_profile) should have different ingestion time than batched logs")
}

// TestAppSignalsMetricsRouting verifies that the routing connector correctly
// splits metrics between the EMF and OTLP monitoring destinations:
//   - Application Signals metrics (Latency, Error, Fault) → default pipeline → EMF exporter
//     → validated by checking /aws/application-signals/data log group for metric content
//   - Metrics with datapoint attributes["Telemetry.Source"] == "ServiceEvents"
//     → OTLP monitoring endpoint → validated by PromQL query confirming ingestion
func TestAppSignalsMetricsRouting(t *testing.T) {
	emfLogGroup := "/aws/application-signals/data"

	startLogsAgent(t)
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	// Send Application Signals metrics (Latency, Error, Fault) using the standard
	// server_consumer.json payload which matches EMF metric_declarations
	sendMetrics(t, "resources/metrics/server_consumer.json", 3, "")

	// Send ServiceEvents metrics → OTLP monitoring endpoint
	sendMetrics(t, "service_events_metric", 3, "ServiceEvents")

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	agentLog, _ := os.ReadFile(common.AgentLogFile)
	agentLogStr := string(agentLog)

	t.Run("no_export_errors", func(t *testing.T) {
		assert.NotContains(t, agentLogStr, "Exporting failed. Dropping data",
			"Agent should not drop metrics data — both routing paths should succeed")
	})

	// EMF validation: Application Signals metrics should appear in EMF log group
	t.Run("appsignals_metrics_in_emf_log_group", func(t *testing.T) {
		streams := awsservice.GetLogStreams(emfLogGroup)
		require.NotEmpty(t, streams, "EMF log group %s should have log streams", emfLogGroup)

		found := false
		for _, stream := range streams {
			events, err := awsservice.GetLogsSince(emfLogGroup, *stream.LogStreamName, &start, &end)
			if err != nil {
				continue
			}
			for _, event := range events {
				if strings.Contains(*event.Message, "Latency") || strings.Contains(*event.Message, "Error") || strings.Contains(*event.Message, "Fault") {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		assert.True(t, found,
			"Application Signals metrics (Latency/Error/Fault) should appear in EMF log group %s", emfLogGroup)
	})

	// OTLP validation: otlphttpexporter debug log shows request to monitoring endpoint
	t.Run("service_events_sent_to_otlp_monitoring_endpoint", func(t *testing.T) {
		assert.Contains(t, agentLogStr,
			`"Preparing to make HTTP request","url":"https://monitoring.us-east-1.amazonaws.com/v1/metrics"`,
			"otlphttpexporter should log HTTP request to monitoring endpoint for ServiceEvents metrics")
	})

	// OTLP validation: query the PromQL API to confirm the metric was actually ingested
	t.Run("service_events_metric_queryable_via_promql", func(t *testing.T) {
		resp, err := awsservice.QueryOtlpMetricsWithRetry(
			"us-east-1",
			`service_events_metric{}`,
			5,
			30*time.Second,
		)
		require.NoError(t, err, "PromQL query for service_events_metric should succeed")
		assert.Equal(t, "success", resp.Status)
		assert.NotEmpty(t, resp.Data.Result,
			"service_events_metric should be queryable via CW OTLP PromQL API after export")
	})

	// ServiceEvents metrics should NOT be in EMF log group (routed to OTLP instead)
	t.Run("service_events_not_in_emf_log_group", func(t *testing.T) {
		streams := awsservice.GetLogStreams(emfLogGroup)
		if len(streams) == 0 {
			return
		}
		for _, stream := range streams {
			events, err := awsservice.GetLogsSince(emfLogGroup, *stream.LogStreamName, &start, &end)
			if err != nil {
				continue
			}
			for _, event := range events {
				assert.NotContains(t, *event.Message, "service_events_metric",
					"service_events_metric should NOT appear in EMF log group")
			}
		}
	})
}

// --- Helpers ---

func sendOTLPLogs(t *testing.T, serviceName string, numLogs int) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_logs.sh",
		serviceName, fmt.Sprintf("%d", numLogs), testHostName, testInstanceID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_logs.sh for %s returned error: %v\nOutput: %s", serviceName, err, string(output))
	}
}

func sendOTLPLogsNoService(t *testing.T, numLogs int) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_logs_no_service.sh", fmt.Sprintf("%d", numLogs))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_logs_no_service.sh returned error: %v\nOutput: %s", err, string(output))
	}
}

func sendAggregateProfileLogs(t *testing.T, serviceName string, numLogs int) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_log_aggregate_profile.sh",
		serviceName, fmt.Sprintf("%d", numLogs), testHostName, testInstanceID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_log_aggregate_profile.sh for %s returned error: %v\nOutput: %s", serviceName, err, string(output))
	}
}

func sendMetrics(t *testing.T, metricName string, numDatapoints int, telemetrySource string) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_metrics.sh",
		metricName, fmt.Sprintf("%d", numDatapoints), telemetrySource)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_metrics.sh for %s returned error: %v\nOutput: %s", metricName, err, string(output))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...\n" + s[len(s)-n:]
}
