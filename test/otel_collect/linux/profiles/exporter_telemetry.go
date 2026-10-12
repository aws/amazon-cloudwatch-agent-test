// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// Must match agent.self_telemetry.port in profiles_config.json.
	selfTelemetryPort = 8888

	// The exporter helper counts a profile sample as sent only after the backend answered 2xx
	// (retries included), and as send-failed only after retries are exhausted or the error is
	// permanent. Samples still being retried appear in neither counter.
	sentProfileSamplesMetric       = "otelcol_exporter_sent_profile_samples"
	sendFailedProfileSamplesMetric = "otelcol_exporter_send_failed_profile_samples"
)

var selfTelemetryURL = fmt.Sprintf("http://127.0.0.1:%d/metrics", selfTelemetryPort)

type exporterCounts struct {
	sent       float64
	sendFailed float64
	found      bool
}

// Scoped to the profiles exporter so other exporters sharing the endpoint cannot mask a profiles
// failure.
func scrapeProfilesExporterCounts() (exporterCounts, error) {
	return scrapeExporterCounts(profilesExporterID, sentProfileSamplesMetric, sendFailedProfileSamplesMetric)
}

func scrapeExporterCounts(exporterID, sentMetric, failedMetric string) (exporterCounts, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(selfTelemetryURL)
	if err != nil {
		return exporterCounts{}, fmt.Errorf("GET %s failed: %w", selfTelemetryURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return exporterCounts{}, fmt.Errorf("GET %s returned %d: %s", selfTelemetryURL, resp.StatusCode, string(body))
	}
	return parseExporterCounts(resp.Body, exporterID, sentMetric, failedMetric)
}

func parseProfilesExporterCounts(r io.Reader) (exporterCounts, error) {
	return parseExporterCounts(r, profilesExporterID, sentProfileSamplesMetric, sendFailedProfileSamplesMetric)
}

// Hand-written because prometheus/common/expfmt is not a dependency of this module and only these
// two counters are needed. A _total suffix is accepted and series with other labels are summed.
func parseExporterCounts(r io.Reader, exporterID, sentMetric, failedMetric string) (exporterCounts, error) {
	var counts exporterCounts
	exporterLabel := fmt.Sprintf(`exporter="%s"`, exporterID)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || !strings.Contains(line, exporterLabel) {
			continue
		}
		name, value, ok := splitSample(line)
		if !ok {
			continue
		}
		switch strings.TrimSuffix(name, "_total") {
		case sentMetric:
			counts.sent += value
			counts.found = true
		case failedMetric:
			counts.sendFailed += value
			counts.found = true
		}
	}
	return counts, scanner.Err()
}

func splitSample(line string) (string, float64, bool) {
	brace := strings.IndexByte(line, '{')
	closing := strings.LastIndexByte(line, '}')
	if brace <= 0 || closing < brace {
		return "", 0, false
	}
	fields := strings.Fields(line[closing+1:])
	if len(fields) == 0 {
		return "", 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", 0, false
	}
	return line[:brace], value, true
}
