// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package profiling

import (
	"log"
	"os"
	"runtime/trace"
	"testing"
)

const (
	envEnableProfiling = "CWA_TEST_PROFILING"
	defaultReportPath  = "/tmp/cwa_test_profile.json"
	defaultTracePath   = "/tmp/cwa_test_trace.out"
)

// Enabled returns true if CWA_TEST_PROFILING=1 is set.
func Enabled() bool {
	return os.Getenv(envEnableProfiling) == "1"
}

// RunWithProfiling wraps m.Run() with profiling and tracing when enabled.
// Use in TestMain:
//
//	func TestMain(m *testing.M) {
//	    os.Exit(profiling.RunWithProfiling(m))
//	}
func RunWithProfiling(m *testing.M) int {
	if !Enabled() {
		return m.Run()
	}

	// Start runtime/trace for goroutine, network, and syscall visibility
	traceFile, err := os.Create(defaultTracePath)
	if err != nil {
		log.Printf("profiling: failed to create trace file: %v", err)
		return m.Run()
	}
	defer traceFile.Close()

	if err := trace.Start(traceFile); err != nil {
		log.Printf("profiling: failed to start trace: %v", err)
		return m.Run()
	}

	code := m.Run()

	trace.Stop()

	prof := Global()

	// Write local report file
	reportPath := os.Getenv("CWA_TEST_PROFILE_PATH")
	if reportPath == "" {
		reportPath = defaultReportPath
	}
	if err := prof.WriteReport(reportPath); err != nil {
		log.Printf("profiling: failed to write report: %v", err)
	}

	// Emit JSON between markers so GHA runner can parse from Terraform output
	prof.EmitSummary()

	return code
}
