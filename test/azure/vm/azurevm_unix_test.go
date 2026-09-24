// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration && !windows

package vm

var platformMetrics = []string{
	"system.cpu.frequency",
	"system.disk.merged",
	"system.disk.weighted_io_time",
	"system.filesystem.inodes.usage",
	"system.linux.memory.available",
	"system.linux.memory.dirty",
	"system.processes.count",
	"system.processes.created",
}

// spanMetrics are the spanmetrics connector's metrics, derived from the pushed spans. Emitted on Linux
// only (the Windows runs produce none), and validated on @resource.* labels alone because the connector
// scope carries no cloudwatch.source/solution.
var spanMetrics = []string{
	"traces.span.metrics.calls",
	"traces.span.metrics.duration",
}
