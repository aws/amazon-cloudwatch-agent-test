// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfilesExporterLogProblem(t *testing.T) {
	require.False(t, profilesExporterLogProblem(
		`I! {"msg":"Exporting failed. Will retry the request after interval.","otelcol.component.id":"otlp_http/profiles"}`))

	require.True(t, profilesExporterLogProblem(
		`E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/profiles"}`))

	require.True(t, profilesExporterLogProblem(
		`W! Rejecting data for profiles pipeline`))

	require.True(t, profilesExporterLogProblem(
		`W! {"msg":"Partial success response","otelcol.component.id":"otlp_http/profiles"}`))

	require.False(t, profilesExporterLogProblem(
		`E! {"msg":"Exporting failed. Dropping data.","otelcol.component.id":"otlp_http/metrics"}`))

	require.False(t, profilesExporterLogProblem(`I! profiles exporter started`))
}

func TestExporterDeliveryError(t *testing.T) {
	require.NoError(t, exporterDeliveryError(exporterCounts{sent: 4, found: true}, 4))

	require.NoError(t, exporterDeliveryError(exporterCounts{sent: 4, sendFailed: 0, found: true}, 4))

	err := exporterDeliveryError(exporterCounts{sent: 3, sendFailed: 1, found: true}, 4)
	require.ErrorContains(t, err, "regular pushes")
	require.ErrorContains(t, err, "failed to send")

	err = exporterDeliveryError(exporterCounts{found: true}, 4)
	require.ErrorContains(t, err, "sent no profile samples")

	err = exporterDeliveryError(exporterCounts{sent: 2, found: true}, 4)
	require.ErrorContains(t, err, "fewer than the 4 payloads")
}
