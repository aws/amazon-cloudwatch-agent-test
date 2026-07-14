// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration

package azurevm

import (
	"fmt"
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

// buildTracesPayload emits an OTLP span whose instance_id attribute becomes the X-Ray annotation the test filters on.
func buildTracesPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	start := now - int64(time.Second)
	// X-Ray requires 16-byte trace / 8-byte span ids; embed a rolling suffix so ids vary across pushes.
	traceID := fmt.Sprintf("%032x", now)
	spanID := fmt.Sprintf("%016x", now)
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
}`, serviceName, instanceID, traceID, spanID, start, now, instanceID))
}
