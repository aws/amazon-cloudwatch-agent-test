// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultServiceNameError(t *testing.T) {
	require.NoError(t, defaultServiceNameError(exporterCounts{sent: 12}, exporterCounts{sent: 13}, ""))

	err := defaultServiceNameError(exporterCounts{sent: 12}, exporterCounts{sent: 12}, "")
	require.ErrorContains(t, err, "no samples sent")

	err = defaultServiceNameError(exporterCounts{sent: 12}, exporterCounts{sent: 12, sendFailed: 1}, "E! Exporting failed ... 400")
	require.ErrorContains(t, err, "resource/profiles")
	require.ErrorContains(t, err, "400")

	failing := "E! Exporting failed. Dropping data. ... 404 Operation not supported"
	err = defaultServiceNameError(exporterCounts{sendFailed: 12}, exporterCounts{sendFailed: 13}, failing)
	require.ErrorContains(t, err, "export itself is failing")
	require.ErrorContains(t, err, "404 Operation not supported")
	require.NotContains(t, err.Error(), "resource/profiles")
}

func TestLastExportFailure(t *testing.T) {
	lines := []string{
		"I! profiles exporter started",
		"I! Exporting failed. Will retry the request after interval. profiles ... 503",
		"E! Exporting failed. Dropping data. profiles ... 404",
		"I! profiles still running",
	}
	require.Contains(t, lastExportFailure(lines), "404")
	require.Empty(t, lastExportFailure(lines[:1]))
}
