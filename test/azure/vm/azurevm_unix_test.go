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
	"traces.span.metrics.calls",
	"traces.span.metrics.duration",
}
