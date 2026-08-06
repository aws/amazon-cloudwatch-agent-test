// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

package vm

import (
	"fmt"
	"sync"
	"time"
)

// startTimeNano is captured once so counter data points share a stable start time across the run.
var startTimeNano = time.Now().UnixNano()

// buildMetricsPayload emits a monotonic counter and a gauge tagged with this host's instance id so the
// CloudWatch OTLP PromQL query can isolate them. service.name lets the collector derive a stable stream/scope.
func buildMetricsPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	return []byte(fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeMetrics": [{
      "scope": {"name": "azurevm-otlp-test-metrics", "version": "1.0.0"},
      "metrics": [
        {
          "name": "azurevm_otlp_counter",
          "unit": "1",
          "sum": {
            "aggregationTemporality": 2,
            "isMonotonic": true,
            "dataPoints": [{"asInt": "1", "startTimeUnixNano": "%d", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        },
        {
          "name": "azurevm_otlp_gauge",
          "unit": "1",
          "gauge": {
            "dataPoints": [{"asDouble": 42.0, "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        }
      ]
    }]
  }]
}`, serviceName, instanceID, startTimeNano, now, instanceID, now, instanceID))
}

// buildLogsPayload emits an OTLP log whose body carries a per-host marker for CloudWatch Logs validation.
func buildLogsPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	body := fmt.Sprintf("azurevm_otlp_log_%s", instanceID)
	return []byte(fmt.Sprintf(`{
  "resourceLogs": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeLogs": [{
      "scope": {"name": "azurevm-otlp-test-logs"},
      "logRecords": [{
        "timeUnixNano": "%d",
        "severityText": "INFO",
        "body": {"stringValue": "%s"},
        "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, serviceName, instanceID, now, body, instanceID))
}

// traceSeq is an incrementing counter ensuring unique trace/span IDs across calls.
var traceSeq uint64

// traceMu protects traceSeq and generatedTraceIDs which are written by the sendTelemetry
// goroutine and read by the test goroutine after the load window closes.
var traceMu sync.Mutex

// generatedTraceIDs collects the OTLP trace IDs (32 hex chars) the collector ACCEPTED during the
// load window -- see recordTraceID.
// validateTraces queries the aws/spans log group (Transaction Search) for these exact IDs.
var generatedTraceIDs []string

// buildTracesPayload emits an OTLP span whose trace ID follows the X-Ray format: the first 4 bytes hold
// the Unix epoch in seconds, matching what the X-Ray propagator's own ID generator does and what every
// other trace producer in this repo does. X-Ray rejects IDs whose embedded date is too far in the past
// with InvalidTraceId, and randomly generated IDs were silently dropped during bring-up.
//
// Transaction Search stores spans with W3C trace IDs, so it is possible this prefix is no longer
// required on the ingest path -- that has not been re-verified. Keeping it is valid either way.
func buildTracesPayload(instanceID string) ([]byte, string) {
	traceMu.Lock()
	traceSeq++
	now := time.Now()
	nowNano := now.UnixNano()
	startNano := nowNano - int64(time.Second)
	// First 4 bytes: unix seconds (X-Ray requirement). Remaining 12 bytes: sequence + padding for uniqueness.
	traceID := fmt.Sprintf("%08x0000000000000000%08x", now.Unix(), traceSeq)
	spanID := fmt.Sprintf("%016x", nowNano)
	traceMu.Unlock()
	return []byte(fmt.Sprintf(`{
  "resourceSpans": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeSpans": [{
      "scope": {"name": "azurevm-otlp-test-traces"},
      "spans": [{
        "traceId": "%s",
        "spanId": "%s",
        "name": "azurevm-otlp-test-span",
        "kind": 2,
        "startTimeUnixNano": "%d",
        "endTimeUnixNano": "%d",
        "attributes": [{"key": "instance_id", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, serviceName, instanceID, traceID, spanID, startNano, nowNano, instanceID)), traceID
}

// recordTraceID marks a trace ID as successfully delivered so validateTraces expects to find it.
func recordTraceID(traceID string) {
	traceMu.Lock()
	defer traceMu.Unlock()
	generatedTraceIDs = append(generatedTraceIDs, traceID)
}
