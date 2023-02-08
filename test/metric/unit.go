// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

type MetricUnit string

const (
	SECONDS        MetricUnit = "Seconds"
	MICROSECONDS   MetricUnit = "Microseconds"
	MILLISECONDS   MetricUnit = "Milliseconds"
	NONE           MetricUnit = "None"
	COUNT          MetricUnit = "Count"
	PERCENT        MetricUnit = "Percent"
	COUNTPERSECOND MetricUnit = "Count/Second"
	BYTES          MetricUnit = "Bytes"
	UNSPECIFIED    MetricUnit = "Unspecified"
)
