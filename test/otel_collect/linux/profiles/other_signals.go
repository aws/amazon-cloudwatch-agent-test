// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	metricsURLPath = "/v1/metrics"

	sentMetricPointsMetric       = "otelcol_exporter_sent_metric_points"
	sendFailedMetricPointsMetric = "otelcol_exporter_send_failed_metric_points"

	failOpenMetricName = "cwagent_integ_test_profiles_fail_open"
)

type otherSignalsSnapshot struct {
	counts exporterCounts
	err    error
	pid    string
	read   bool
}

func metricsExporterID(content []byte) (string, error) {
	var cfg collectorConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("parsing translated config: %w", err)
	}
	var ids []string
	for name, p := range cfg.Service.Pipelines {
		if !strings.HasPrefix(name, "metrics/") {
			continue
		}
		for _, id := range p.Exporters {
			if typ := componentType(id); typ == "otlphttp" || typ == "otlp_http" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	sort.Strings(ids)
	return ids[0], nil
}

const (
	metricsDelivered       = "metrics delivered"
	metricsBackendRejected = "metrics pipeline active, backend rejects metrics in this account (403)"
)

// otherSignalsOutcome judges the fail-open check: while profiles were being exported, the metrics
// pipeline kept working and the agent process was not replaced. It passes when the metrics
// exporter delivered points, or when every point failed only because the backend rejects metrics
// from this account (403), which says nothing about profiles. Any other export error fails it.
// exportErrors are the metrics exporter's failed-export log lines. A crash and restart shows as
// a changed process ID or as counters that went backwards.
func otherSignalsOutcome(exporterID string, start, end otherSignalsSnapshot, metricsAccepted int, exportErrors []string) (string, error) {
	switch {
	case !start.read || !end.read:
		return "", fmt.Errorf("self telemetry was not read at both ends of the push window")
	case start.err != nil:
		return "", fmt.Errorf("reading %s counters at the start: %w", exporterID, start.err)
	case end.err != nil:
		return "", fmt.Errorf("reading %s counters at the end (agent down?): %w", exporterID, end.err)
	case start.pid != "" && end.pid != "" && start.pid != end.pid:
		return "", fmt.Errorf("agent process changed from pid %s to %s during the run", start.pid, end.pid)
	case metricsAccepted == 0:
		return "", fmt.Errorf("no metrics payload was accepted by the agent's OTLP receiver")
	case start.counts.found && !end.counts.found:
		return "", fmt.Errorf("%s %s series disappeared during the run (agent restarted?)", exporterID, sentMetricPointsMetric)
	case !end.counts.found:
		return "", fmt.Errorf("self telemetry has no %s series for %s", sentMetricPointsMetric, exporterID)
	case end.counts.sent < start.counts.sent || end.counts.sendFailed < start.counts.sendFailed:
		return "", fmt.Errorf("%s counters went backwards (sent %.0f to %.0f, failed %.0f to %.0f): the agent restarted",
			exporterID, start.counts.sent, end.counts.sent, start.counts.sendFailed, end.counts.sendFailed)
	case end.counts.sent > start.counts.sent:
		return metricsDelivered, nil
	case end.counts.sendFailed == start.counts.sendFailed:
		return "", fmt.Errorf("%s handled no metric points during the run (sent %.0f, failed %.0f)", exporterID, end.counts.sent, end.counts.sendFailed)
	}
	failed := end.counts.sendFailed - start.counts.sendFailed
	if len(exportErrors) == 0 {
		return "", fmt.Errorf("%s failed to send %.0f metric points and the agent log names no export error", exporterID, failed)
	}
	for _, line := range exportErrors {
		if !isBackendRejection(line) {
			return "", fmt.Errorf("%s failed to send %.0f metric points with an error other than a backend rejection: %s", exporterID, failed, line)
		}
	}
	return metricsBackendRejected, nil
}

// backendRejectionMarkers identify an export the metrics backend refused for this account
// (HTTP 403, mapped to gRPC PermissionDenied), as opposed to network, timeout or 5xx failures.
var backendRejectionMarkers = []string{"Status Code 403", "PermissionDenied", "not allowed to publish"}

func isBackendRejection(line string) bool {
	for _, marker := range backendRejectionMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func exporterFailureLines(exporterID string) ([]string, error) {
	content, err := common.RunCommand("sudo cat " + common.AgentLogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent log %s: %w", common.AgentLogFile, err)
	}
	return filterExportFailures(strings.Split(content, "\n"), exporterID), nil
}

func filterExportFailures(lines []string, exporterID string) []string {
	component := fmt.Sprintf("%q:%q", componentIDLogKey, exporterID)
	var failures []string
	for _, line := range lines {
		if strings.Contains(line, component) && strings.Contains(strings.ToLower(line), exportFailedLogText) {
			failures = append(failures, line)
		}
	}
	return failures
}

func buildMetricsPayload(serviceName string) []byte {
	payload := map[string]any{
		"resourceMetrics": []any{map[string]any{
			"resource": map[string]any{"attributes": []resourceAttr{attr(serviceNameKey, serviceName)}},
			"scopeMetrics": []any{map[string]any{
				"scope": map[string]any{"name": "cloudwatch-agent-integ-test", "version": "1.0.0"},
				"metrics": []any{map[string]any{
					"name": failOpenMetricName,
					"unit": "1",
					"gauge": map[string]any{"dataPoints": []any{map[string]any{
						"timeUnixNano": fmt.Sprint(time.Now().UnixNano()),
						"asDouble":     1,
					}}},
				}},
			}},
		}},
	}
	out, err := json.Marshal(payload)
	if err != nil {
		// Every value above is a plain map, slice or string, so marshalling cannot fail.
		panic(err)
	}
	return out
}

func postMetrics(payload []byte) error {
	client := &http.Client{Timeout: pushTimeout}
	resp, err := client.Post(otlpHTTPEndpoint+metricsURLPath, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST %s failed: %w", metricsURLPath, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s returned %d: %s", metricsURLPath, resp.StatusCode, string(body))
	}
	return nil
}

func agentPID() string {
	out, err := common.RunCommand("systemctl show -p MainPID --value amazon-cloudwatch-agent")
	pid := strings.TrimSpace(out)
	if err != nil || pid == "" || pid == "0" {
		return ""
	}
	return pid
}
