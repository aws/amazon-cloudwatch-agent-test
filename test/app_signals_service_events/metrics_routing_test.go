// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Tests for the Application Signals OTLP metrics routing: the routing connector
// splits metrics between the EMF destination and the OTLP monitoring endpoint.
package app_signals_service_events

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/test/otlp_export/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

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
		var found bool
		for attempt := 0; attempt < 6; attempt++ {
			streams := awsservice.GetLogStreams(emfLogGroup)
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
			if found {
				break
			}
			time.Sleep(30 * time.Second)
		}
		assert.True(t, found,
			"Application Signals metrics (Latency/Error/Fault) should appear in EMF log group %s", emfLogGroup)
	})

	// OTLP validation: otlphttpexporter debug log shows request to monitoring endpoint
	t.Run("service_events_sent_to_otlp_monitoring_endpoint", func(t *testing.T) {
		assert.Contains(t, agentLogStr,
			`"Preparing to make HTTP request","url":"https://monitoring.us-west-2.amazonaws.com/v1/metrics"`,
			"otlphttpexporter should log HTTP request to monitoring endpoint for ServiceEvents metrics")
	})

	// OTLP validation: query the PromQL API to confirm the metric was actually ingested
	t.Run("service_events_metric_queryable_via_promql", func(t *testing.T) {
		result := otlpvalidation.ValidateOtlpMetrics(
			"service_events_routing", "us-west-2", []string{"service_events_metric"},
		)
		for _, r := range result.TestResults {
			assert.Equal(t, status.SUCCESSFUL, r.Status,
				"metric %s should be queryable via CW OTLP PromQL API: %v", r.Name, r.Reason)
		}
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
