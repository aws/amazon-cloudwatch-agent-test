// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package windows

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
)

func RunNVIDIATest() error {
	// Test NEEDS_SETUP phase
	if err := checkNVIDIAStatus(util.NVIDIASetupStatus); err != nil {
		return fmt.Errorf("initial NVIDIA status check failed: %v", err)
	}

	// Test READY phase
	if err := installNVIDIADriver(); err != nil {
		return fmt.Errorf("failed to install NVIDIA driver: %v", err)
	}
	if err := checkNVIDIAStatus(util.Ready); err != nil {
		return fmt.Errorf("post-install NVIDIA status check failed: %v", err)
	}

	// Test NVIDIA + process
	if err := testCombinedWorkloads(); err != nil {
		return fmt.Errorf("combined workload test failed: %v", err)
	}

	if err := uninstallNVIDIADriver(); err != nil {
		return fmt.Errorf("uninstall nvidia driver failed: %v", err)
	}

	return nil
}

func uninstallNVIDIADriver() error {
	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Uninstall-NvidiaDriver")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installNVIDIADriver failed: %v, output: %s", err, string(output))
	}
	return nil
}

func installNVIDIADriver() error {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Install-NvidiaDriver", "-Bucket", env.Bucket)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installNVIDIADriver failed: %v, output: %s", err, string(output))
	}
	return nil
}

func checkNVIDIAStatus(expectedStatus string) error {
	workloads, err := util.GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	for _, workload := range workloads {
		if util.Contains(workload.Categories, "NVIDIA_GPU") && workload.Status == expectedStatus {
			return nil
		}
	}

	return fmt.Errorf("NVIDIA GPU workload with status %s not found", expectedStatus)
}

func testCombinedWorkloads() error {
	if err := util.CreateTestJAR("C:\\tmp\\simple-test.jar", map[string]string{
		"Main-Class": "Main",
	}); err != nil {
		return fmt.Errorf("failed to create test JAR: %v", err)
	}
	defer os.Remove("C:\\tmp\\simple-test.jar")

	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Start-JVM", "-JarPath", "C:\\tmp\\simple-test.jar", "-Port", strconv.Itoa(JVMPort))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to start JVM with port: %v", err)
	}
	pid := strings.TrimSpace(string(output))
	defer exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-JVM", "-ProcessId", pid).Run()

	log.Printf("Started JVM process with PID: %s", pid)
	time.Sleep(util.WorkloadUptimeSleep)
	return checkCombinedWorkloads()
}

func checkCombinedWorkloads() error {
	workloads, err := util.GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	hasNVIDIA := false
	hasJVM := false

	for _, workload := range workloads {
		if util.Contains(workload.Categories, "NVIDIA_GPU") && workload.Status == "READY" {
			hasNVIDIA = true
			log.Printf("Found NVIDIA GPU workload: %s", workload.Name)
		}
		if util.Contains(workload.Categories, "JVM") && workload.Status == "READY" {
			hasJVM = true
			log.Printf("Found JVM workload: %s", workload.Name)
		}
	}

	if !hasNVIDIA {
		return fmt.Errorf("NVIDIA GPU workload not found in combined test")
	}

	if !hasJVM {
		return fmt.Errorf("JVM workload not found in combined test")
	}

	log.Printf("Successfully detected both NVIDIA GPU and JVM workloads")
	return nil
}
