// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProfilesExporterCounts(t *testing.T) {
	text := `# HELP otelcol_exporter_sent_profile_samples Number of profile samples successfully sent.
# TYPE otelcol_exporter_sent_profile_samples counter
otelcol_exporter_sent_profile_samples{NodeName="i-0",exporter="otlp_http/profiles"} 15
otelcol_exporter_sent_metric_points{NodeName="i-0",exporter="otlp_http/metrics"} 99
otelcol_exporter_send_failed_profile_samples{NodeName="i-0",exporter="otlp_http/profiles"} 2
otelcol_exporter_send_failed_profile_samples{NodeName="i-0",exporter="otlp_http/other"} 7
`
	counts, err := parseProfilesExporterCounts(strings.NewReader(text))
	require.NoError(t, err)
	require.True(t, counts.found)
	require.Equal(t, 15.0, counts.sent)
	require.Equal(t, 2.0, counts.sendFailed)
}

func TestParseProfilesExporterCountsAbsent(t *testing.T) {
	counts, err := parseProfilesExporterCounts(strings.NewReader(
		`otelcol_exporter_sent_metric_points{exporter="otlp_http/metrics"} 1` + "\n"))
	require.NoError(t, err)
	require.False(t, counts.found)
}
