// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfilesExporterLogProblem(t *testing.T) {
	// A retried export logged at info level is not a problem on its own.
	require.False(t, profilesExporterLogProblem(
		`I! {"msg":"Exporting failed. Will retry the request after interval.","otelcol.component.id":"otlp_http/profiles"}`))

	require.True(t, profilesExporterLogProblem(
		`E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/profiles"}`))

	// A reject line for the profiles exporter is a problem even without the E! prefix.
	require.True(t, profilesExporterLogProblem(
		`W! Rejecting data for profiles pipeline`))

	// A partial success response for profiles fails the main test (the attribute cap runner is
	// where a partial success is expected).
	require.True(t, profilesExporterLogProblem(
		`W! {"msg":"Partial success response","otelcol.component.id":"otlp_http/profiles"}`))

	// A line that does not mention profiles is ignored, even at error level.
	require.False(t, profilesExporterLogProblem(
		`E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/metrics"}`))

	require.False(t, profilesExporterLogProblem(`I! profiles exporter started`))
}

func TestExporterDeliveryError(t *testing.T) {
	require.NoError(t, exporterDeliveryError(exporterCounts{sent: 4, found: true}, 4))

	// A probe-only failure is not visible here: the regular-push snapshot is clean, so delivery
	// passes. The probe is judged by Profiles_DefaultServiceName.
	require.NoError(t, exporterDeliveryError(exporterCounts{sent: 4, sendFailed: 0, found: true}, 4))

	err := exporterDeliveryError(exporterCounts{sent: 3, sendFailed: 1, found: true}, 4)
	require.ErrorContains(t, err, "regular pushes")
	require.ErrorContains(t, err, "failed to send")

	err = exporterDeliveryError(exporterCounts{found: true}, 4)
	require.ErrorContains(t, err, "sent no profile samples")

	err = exporterDeliveryError(exporterCounts{sent: 2, found: true}, 4)
	require.ErrorContains(t, err, "fewer than the 4 payloads")
}
