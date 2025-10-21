// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package windows

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
)

const (
	numJVMs           = 5
	numRandomProcs    = 20
	binaryPathWindows = `C:\Program Files\Amazon\AmazonCloudWatchAgent\workload-discovery.exe`
	sizeLimitBytes    = 3 * 1024 * 1024
	memLimitBytes     = 15 * 1024 * 1024
	cpuPercentLimit   = 25.0
	latencyLimitSecs  = 5.0
)

func RunPerformanceTest() error {
	jarPath := `C:\tmp\perf-test.jar`
	if err := util.CreateTestJAR(jarPath, map[string]string{"Main-Class": "Main"}); err != nil {
		return fmt.Errorf("create test jar: %w", err)
	}
	defer os.Remove(jarPath)

	var jvmPIDs []string
	for i := 0; i < numJVMs; i++ {
		cmd := exec.Command("powershell", "-NoProfile", "-File", "C:\\scripts.ps1", "Start-JVM", "-JarPath", jarPath)
		out, err := cmd.Output()
		if err != nil {
			stopJVMs(jvmPIDs)
			return fmt.Errorf("start-jvm %d failed: %w", i, err)
		}
		pid := strings.TrimSpace(string(out))
		jvmPIDs = append(jvmPIDs, pid)
	}

	var randPIDs []int
	for i := 0; i < numRandomProcs; i++ {
		rc := exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 60")
		if err := rc.Start(); err != nil {
			stopJVMs(jvmPIDs)
			killRand(randPIDs)
			return fmt.Errorf("start random process %d: %w", i, err)
		}
		if rc.Process != nil {
			randPIDs = append(randPIDs, rc.Process.Pid)
		}
	}

	info, err := os.Stat(binaryPathWindows)
	if err != nil {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("stat workload-discovery: %w", err)
	}
	if info.Size() >= sizeLimitBytes {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("binary too large: %d bytes (limit %d)", info.Size(), sizeLimitBytes)
	}

	outFile := filepath.Join(os.TempDir(), "wd_out.txt")
	ps := buildPerfPS(binaryPathWindows, outFile)
	start := time.Now()
	out, runErr := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	elapsedGo := time.Since(start).Seconds()

	wdo, _ := os.ReadFile(outFile)
	fmt.Printf("workload-discovery output:\n%s\n", string(wdo))
	_ = os.Remove(outFile)

	if runErr != nil {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("run workload-discovery: %w", runErr)
	}

	var m psMetrics
	if err := json.Unmarshal(out, &m); err != nil {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("parse metrics json: %w (raw: %s)", err, string(out))
	}

	numCores, err := getNumCPUCores()
	if err != nil {
		numCores = 1
	}

	elapsed := elapsedGo
	cpuEfficiency := (m.CPUSeconds / elapsed) * 100.0
	systemCpuPct := cpuEfficiency / float64(numCores)

	if elapsed >= latencyLimitSecs {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("latency too high: %.3f s (limit %.3f s)", elapsed, latencyLimitSecs)
	}
	if systemCpuPct >= cpuPercentLimit {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("system CPU usage too high: %.2f%% (limit %.2f%%)", systemCpuPct, cpuPercentLimit)
	}
	if m.PeakWorkingSet >= memLimitBytes {
		stopJVMs(jvmPIDs)
		killRand(randPIDs)
		return fmt.Errorf("memory usage too high: %d bytes (limit %d bytes)", m.PeakWorkingSet, memLimitBytes)
	}

	stopJVMs(jvmPIDs)
	killRand(randPIDs)
	return nil
}

type psMetrics struct {
	CPUSeconds     float64 `json:"cpu"`
	PeakWorkingSet int64   `json:"peak"`
	Duration       float64 `json:"duration"`
}

func buildPerfPS(exe, outFile string) string {
	s := []string{
		"$ErrorActionPreference='Stop';",
		"$exe = '" + escapePS(exe) + "';",
		"$out = '" + escapePS(outFile) + "';",
		"$sw = [System.Diagnostics.Stopwatch]::StartNew();",
		"$p = Start-Process -FilePath $exe -RedirectStandardOutput $out -NoNewWindow -PassThru;",
		"$p.WaitForExit();",
		"$sw.Stop();",
		"$obj = [ordered]@{ cpu = $p.TotalProcessorTime.TotalSeconds; peak = [int64]$p.PeakWorkingSet64; duration = $sw.Elapsed.TotalSeconds };",
		"$obj | ConvertTo-Json -Compress;",
	}
	return strings.Join(s, " ")
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func stopJVMs(pids []string) {
	for _, pid := range pids {
		_ = exec.Command("powershell", "-NoProfile", "-File", "C:\\scripts.ps1", "Stop-JVM", "-ProcessId", pid).Run()
	}
}

func killRand(pids []int) {
	for _, pid := range pids {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
	}
}

func getNumCPUCores() (int, error) {
	cmd := exec.Command("powershell", "-Command", "$env:NUMBER_OF_PROCESSORS")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}
