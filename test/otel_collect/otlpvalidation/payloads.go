// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlpvalidation

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PayloadConfig parameterizes the OTLP payloads shared by the VM integration
// tests. Prefix namespaces every metric, log marker, scope, and span name
// (e.g. "gce" produces gce_otlp_counter), so each platform's telemetry is
// distinguishable in the shared destinations.
type PayloadConfig struct {
	Prefix      string
	ServiceName string
	InstanceID  string
}

// startTimeNano is captured once so counter data points share a stable start time across the run.
var startTimeNano = time.Now().UnixNano()

// MeasuredMetricNames returns the metric names BuildMetricsPayload emits for a prefix.
func MeasuredMetricNames(prefix string) []string {
	return []string{prefix + "_otlp_counter", prefix + "_otlp_gauge"}
}

// LogMarker returns the log body BuildLogsPayload emits: a per-host marker the
// log validation greps for.
func LogMarker(prefix, id string) string {
	return fmt.Sprintf("%s_otlp_log_%s", prefix, id)
}

// BuildMetricsPayload emits a monotonic counter and a gauge tagged with this host's instance id so the
// CloudWatch OTLP PromQL query can isolate them. service.name lets the collector derive a stable stream/scope.
func BuildMetricsPayload(cfg PayloadConfig) []byte {
	now := time.Now().UnixNano()
	return []byte(fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeMetrics": [{
      "scope": {"name": "%s-otlp-test-metrics", "version": "1.0.0"},
      "metrics": [
        {
          "name": "%s_otlp_counter",
          "unit": "1",
          "sum": {
            "aggregationTemporality": 2,
            "isMonotonic": true,
            "dataPoints": [{"asInt": "1", "startTimeUnixNano": "%d", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        },
        {
          "name": "%s_otlp_gauge",
          "unit": "1",
          "gauge": {
            "dataPoints": [{"asDouble": 42.0, "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        }
      ]
    }]
  }]
}`, cfg.ServiceName, cfg.InstanceID, cfg.Prefix, cfg.Prefix, startTimeNano, now, cfg.InstanceID, cfg.Prefix, now, cfg.InstanceID))
}

// BuildLogsPayload emits an OTLP log whose body carries a per-host marker (see LogMarker)
// for CloudWatch Logs validation.
func BuildLogsPayload(cfg PayloadConfig) []byte {
	now := time.Now().UnixNano()
	return []byte(fmt.Sprintf(`{
  "resourceLogs": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeLogs": [{
      "scope": {"name": "%s-otlp-test-logs"},
      "logRecords": [{
        "timeUnixNano": "%d",
        "severityText": "INFO",
        "body": {"stringValue": "%s"},
        "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, cfg.ServiceName, cfg.InstanceID, cfg.Prefix, now, LogMarker(cfg.Prefix, cfg.InstanceID), cfg.InstanceID))
}

// traceSeq is an incrementing counter ensuring unique trace/span IDs across calls.
var traceSeq uint64

// traceSeqMu protects traceSeq, which is incremented from the sender goroutine.
var traceSeqMu sync.Mutex

// BuildTracesPayload emits an OTLP span whose trace ID follows the X-Ray format: the first 4 bytes hold
// the Unix epoch in seconds, matching what the X-Ray propagator's own ID generator does and what every
// other trace producer in this repo does. X-Ray rejects IDs whose embedded date is too far in the past
// with InvalidTraceId, and randomly generated IDs were silently dropped during bring-up.
//
// Transaction Search stores spans with W3C trace IDs, so it is possible this prefix is no longer
// required on the ingest path -- that has not been re-verified. Keeping it is valid either way.
func BuildTracesPayload(cfg PayloadConfig) ([]byte, string) {
	traceSeqMu.Lock()
	traceSeq++
	now := time.Now()
	nowNano := now.UnixNano()
	startNano := nowNano - int64(time.Second)
	// First 4 bytes: unix seconds (X-Ray requirement). Remaining 12 bytes: sequence + padding for uniqueness.
	traceID := fmt.Sprintf("%08x0000000000000000%08x", now.Unix(), traceSeq)
	spanID := fmt.Sprintf("%016x", nowNano)
	traceSeqMu.Unlock()
	return []byte(fmt.Sprintf(`{
  "resourceSpans": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "%s"}},
      {"key": "host.id", "value": {"stringValue": "%s"}}
    ]},
    "scopeSpans": [{
      "scope": {"name": "%s-otlp-test-traces"},
      "spans": [{
        "traceId": "%s",
        "spanId": "%s",
        "name": "%s-otlp-test-span",
        "kind": 2,
        "startTimeUnixNano": "%d",
        "endTimeUnixNano": "%d",
        "attributes": [{"key": "instance_id", "value": {"stringValue": "%s"}}]
      }]
    }]
  }]
}`, cfg.ServiceName, cfg.InstanceID, cfg.Prefix, traceID, spanID, cfg.Prefix, startNano, nowNano, cfg.InstanceID)), traceID
}

// TraceRecorder collects the trace IDs the collector accepted during the load
// window. It is written by the sender goroutine and read by the test goroutine
// after the window closes, so access is mutex-guarded.
type TraceRecorder struct {
	mu  sync.Mutex
	ids []string
}

// Record marks a trace ID as successfully delivered, so trace validation expects to find it.
func (r *TraceRecorder) Record(traceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, traceID)
}

// Snapshot returns a copy of the recorded trace IDs.
func (r *TraceRecorder) Snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.ids))
	copy(out, r.ids)
	return out
}

// PostOTLP sends an OTLP payload and reports whether the collector accepted it.
func PostOTLP(endpoint, path string, payload []byte) bool {
	req, err := http.NewRequest("POST", endpoint+path, bytes.NewReader(payload))
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

// FilterLogLines returns lines from a multi-line string that contain any of the given
// substrings (case-insensitive), capped to the trailing 50 matches.
func FilterLogLines(text string, substrs ...string) []string {
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
