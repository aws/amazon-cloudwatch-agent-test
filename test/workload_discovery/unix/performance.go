// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package unix

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/unix/util"
)

const (
	numJVMs          = 25
	numRandomProcs   = 75
	binaryPathLinux  = "/opt/aws/amazon-cloudwatch-agent/bin/workload-discovery"
	sizeLimitBytes   = 3 * 1024 * 1024
	memLimitKB       = 15 * 1024
	cpuPercentLimit  = 50.0
	latencyLimitSecs = 1.0
)

func RunPerformanceTest() error {
	if err := ensureTimeCommand(); err != nil {
		return nil
	}

	jarPath := "/tmp/perf-test.jar"
	if err := util.CreateTestJAR(jarPath, map[string]string{"Main-Class": "Main"}); err != nil {
		return fmt.Errorf("create test jar: %w", err)
	}
	defer os.Remove(jarPath)

	var jvmPIDs []string
	for i := 0; i < numJVMs; i++ {
		cmd := exec.Command("./unix/util/scripts", "spin_up_jvm", jarPath)
		out, err := cmd.Output()
		if err != nil {
			tearDownJVMs(jvmPIDs)
			return fmt.Errorf("spin_up_jvm %d failed: %w", i, err)
		}
		pid := strings.TrimSpace(string(out))
		jvmPIDs = append(jvmPIDs, pid)
		time.Sleep(1 * time.Second)
	}

	var randCmds []*exec.Cmd
	for i := 0; i < numRandomProcs; i++ {
		rc := exec.Command("sh", "-c", "sleep 60")
		if err := rc.Start(); err != nil {
			tearDownJVMs(jvmPIDs)
			killRand(randCmds)
			return fmt.Errorf("start random process %d: %w", i, err)
		}
		randCmds = append(randCmds, rc)
	}

	info, err := os.Stat(binaryPathLinux)
	if err != nil {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("stat workload-discovery: %w", err)
	}
	if info.Size() >= sizeLimitBytes {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("binary too large: %d bytes (limit %d)", info.Size(), sizeLimitBytes)
	}

	metricsFile := "/tmp/wd_metrics.txt"
	_ = exec.Command("sudo", "rm", "-rf", metricsFile).Run()

	start := time.Now()
	cmd := exec.Command("sudo", "/usr/bin/time", "-f", "%e,%U,%S,%M", "-o", metricsFile, binaryPathLinux)
	output, runErr := cmd.CombinedOutput()
	duration := time.Since(start).Seconds()

	fmt.Printf("workload-discovery output:\n%s\n", string(output))

	if runErr != nil {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("run workload-discovery: %w", runErr)
	}

	elapsed, user, sys, _, err := parseMetrics(metricsFile)
	if err != nil {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("parse metrics: %w", err)
	}

	if duration > 0 {
		elapsed = duration
	}

	numCores, err := getNumCPUCores()
	if err != nil {
		numCores = 1
	}

	den := elapsed
	if den < 1e-9 {
		den = 1e-9
	}
	cpuEfficiency := ((user + sys) / den) * 100.0
	systemCpuPct := cpuEfficiency / float64(numCores)

	if elapsed >= latencyLimitSecs {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("latency too high: %.3f s (limit %.3f s)", elapsed, latencyLimitSecs)
	}
	if systemCpuPct >= cpuPercentLimit {
		tearDown(jvmPIDs, randCmds)
		return fmt.Errorf("system CPU usage too high: %.2f%% (limit %.2f%%)", systemCpuPct, cpuPercentLimit)
	}

	tearDown(jvmPIDs, randCmds)
	return nil
}

func ensureTimeCommand() error {
	if _, err := os.Stat("/usr/bin/time"); err == nil {
		return nil
	}

	fmt.Printf("Installing time command...\n")

	cmd := exec.Command("sudo", "apt", "update")
	if err := cmd.Run(); err == nil {
		cmd = exec.Command("sudo", "apt", "install", "-y", "time")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	cmd = exec.Command("sudo", "dnf", "install", "-y", "time")
	if err := cmd.Run(); err == nil {
		return nil
	}

	cmd = exec.Command("sudo", "yum", "install", "-y", "time")
	if err := cmd.Run(); err == nil {
		return nil
	}

	cmd = exec.Command("sudo", "zypper", "install", "-y", "time")
	if err := cmd.Run(); err == nil {
		return nil
	}

	return fmt.Errorf("could not install time command using apt, dnf, yum, or zypper")
}

func parseMetrics(path string) (elapsed, user, sys float64, maxrssKB int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, 0, 0, errors.New("empty metrics file")
	}
	line := strings.TrimSpace(sc.Text())
	parts := strings.Split(line, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("unexpected metrics format: %q", line)
	}
	elapsed, err = strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return
	}
	user, err = strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return
	}
	sys, err = strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return
	}
	mkb, err := strconv.Atoi(parts[3])
	if err != nil {
		return
	}
	maxrssKB = mkb
	return
}

func tearDownJVMs(pids []string) {
	for _, pid := range pids {
		_ = exec.Command("./unix/util/scripts", "tear_down_jvm", pid).Run()
	}
}

func killRand(cmds []*exec.Cmd) {
	for _, c := range cmds {
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}
}

func tearDown(jpids []string, rcs []*exec.Cmd) {
	tearDownJVMs(jpids)
	killRand(rcs)
	_ = exec.Command("sudo", "rm", "-rf", "/tmp/wd_metrics.txt").Run()
}

func getNumCPUCores() (int, error) {
	cmd := exec.Command("nproc")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}
