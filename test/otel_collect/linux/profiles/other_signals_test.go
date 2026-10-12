// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricsExporterID(t *testing.T) {
	cfg := `
service:
  pipelines:
    metrics/host:
      exporters: [awscloudwatch]
    metrics/opentelemetry:
      exporters: [otlp_http/metrics]
    profiles/otlp:
      exporters: [otlp_http/profiles]
`
	id, err := metricsExporterID([]byte(cfg))
	require.NoError(t, err)
	require.Equal(t, "otlp_http/metrics", id)

	// Only an OTLP HTTP exporter in a metrics pipeline counts.
	id, err = metricsExporterID([]byte(`
service:
  pipelines:
    metrics/host:
      exporters: [awscloudwatch]
    profiles/otlp:
      exporters: [otlp_http/profiles]
`))
	require.NoError(t, err)
	require.Empty(t, id)

	_, err = metricsExporterID([]byte("service: ["))
	require.Error(t, err)
}

const (
	metrics403Line  = `2026-10-01T15:28:55Z E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/metrics","error":"not retryable error: Permanent error: rpc error: code = PermissionDenied desc = error exporting items, request to https://example.invalid/v1/metrics responded with HTTP Status Code 403, Message=This account is not allowed to publish its own service metrics"}`
	metrics503Line  = `2026-10-01T15:28:55Z I! {"msg":"Exporting failed. Will retry the request after interval.","otelcol.component.id":"otlp_http/metrics","error":"rpc error: code = Unavailable desc = error exporting items, request to https://example.invalid/v1/metrics responded with HTTP Status Code 503"}`
	metricsDialLine = `2026-10-01T15:28:55Z E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/metrics","error":"Post \"https://example.invalid/v1/metrics\": dial tcp: i/o timeout"}`
)

func snapshot(sent, failed float64, pid string) otherSignalsSnapshot {
	return otherSignalsSnapshot{counts: exporterCounts{sent: sent, sendFailed: failed, found: true}, pid: pid, read: true}
}

func TestOtherSignalsOutcomeBranches(t *testing.T) {
	const id = "otlp_http/metrics"
	start := snapshot(4, 0, "100")

	// (a) Delivered: sent rose.
	outcome, err := otherSignalsOutcome(id, start, snapshot(20, 0, "100"), 15, nil)
	require.NoError(t, err)
	require.Equal(t, metricsDelivered, outcome)

	// (b) Backend rejects metrics for this account: nothing sent, every failure is a 403.
	outcome, err = otherSignalsOutcome(id, snapshot(0, 3, "100"), snapshot(0, 15, "100"), 15, []string{metrics403Line, metrics403Line})
	require.NoError(t, err)
	require.Equal(t, metricsBackendRejected, outcome)

	// (c) Any other export error fails, even next to 403s.
	for name, line := range map[string]string{"5xx": metrics503Line, "network": metricsDialLine} {
		_, err = otherSignalsOutcome(id, snapshot(0, 3, "100"), snapshot(0, 15, "100"), 15, []string{metrics403Line, line})
		require.Error(t, err, name)
		require.Contains(t, err.Error(), "other than a backend rejection", name)
	}
	_, err = otherSignalsOutcome(id, snapshot(0, 3, "100"), snapshot(0, 15, "100"), 15, nil)
	require.ErrorContains(t, err, "names no export error")
}

func TestOtherSignalsOutcomeFailures(t *testing.T) {
	const id = "otlp_http/metrics"
	start := snapshot(4, 0, "100")
	end := snapshot(20, 0, "100")
	cases := map[string]struct {
		start, end otherSignalsSnapshot
		accepted   int
		want       string
	}{
		"not read":           {otherSignalsSnapshot{}, end, 15, "not read"},
		"scrape failed":      {start, otherSignalsSnapshot{err: errors.New("refused"), read: true}, 15, "agent down"},
		"pid changed":        {start, snapshot(20, 0, "200"), 15, "pid 100 to 200"},
		"none accepted":      {start, end, 0, "no metrics payload"},
		"no series":          {otherSignalsSnapshot{pid: "100", read: true}, otherSignalsSnapshot{pid: "100", read: true}, 15, "no otelcol_exporter_sent_metric_points"},
		"series disappeared": {start, otherSignalsSnapshot{pid: "100", read: true}, 15, "disappeared during the run"},
		"went backwards":     {start, snapshot(2, 0, "100"), 15, "backwards"},
		"failed reset":       {snapshot(4, 9, ""), snapshot(9, 1, ""), 15, "backwards"},
		"stalled":            {start, snapshot(4, 0, "100"), 15, "handled no metric points"},
	}
	for name, c := range cases {
		_, err := otherSignalsOutcome(id, c.start, c.end, c.accepted, nil)
		require.Error(t, err, name)
		require.Contains(t, err.Error(), c.want, name)
	}

	// A process ID that could not be read does not fail the check on its own.
	outcome, err := otherSignalsOutcome(id, start, snapshot(20, 0, ""), 15, nil)
	require.NoError(t, err)
	require.Equal(t, metricsDelivered, outcome)
}

func TestFilterExportFailures(t *testing.T) {
	lines := []string{
		metrics403Line,
		`E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/profiles"}`,
		`I! {"msg":"Everything is ready","otelcol.component.id":"otlp_http/metrics"}`,
		metrics503Line,
	}
	require.Equal(t, []string{metrics403Line, metrics503Line}, filterExportFailures(lines, "otlp_http/metrics"))
	require.True(t, isBackendRejection(metrics403Line))
	require.False(t, isBackendRejection(metrics503Line))
	require.False(t, isBackendRejection(metricsDialLine))
}

func TestBuildMetricsPayload(t *testing.T) {
	payload := buildMetricsPayload("svc-1")
	require.True(t, json.Valid(payload))
	require.Contains(t, string(payload), failOpenMetricName)
	require.Contains(t, string(payload), `"svc-1"`)
}

func TestParseExporterCountsMetricPoints(t *testing.T) {
	text := `otelcol_exporter_sent_metric_points_total{exporter="otlp_http/metrics"} 12
otelcol_exporter_send_failed_metric_points{exporter="otlp_http/metrics"} 1
otelcol_exporter_sent_profile_samples{exporter="otlp_http/profiles"} 5
`
	counts, err := parseExporterCounts(strings.NewReader(text), "otlp_http/metrics", sentMetricPointsMetric, sendFailedMetricPointsMetric)
	require.NoError(t, err)
	require.Equal(t, exporterCounts{sent: 12, sendFailed: 1, found: true}, counts)
}
