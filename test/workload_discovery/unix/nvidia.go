// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package unix

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/unix/util"
)

func RunNVIDIATest() error {
	// Test NEEDS_SETUP phase
	if err := maskNVIDIASmi(); err != nil {
		return fmt.Errorf("failed to mask nvidia-smi: %v", err)
	}
	if err := checkNVIDIAStatus("NEEDS_SETUP/NVIDIA_DRIVER"); err != nil {
		return fmt.Errorf("initial NVIDIA status check failed: %v", err)
	}

	// Test READY phase
	if err := unmaskNVIDIASmi(); err != nil {
		return fmt.Errorf("failed to unmask nvidia-smi: %v", err)
	}
	if err := checkNVIDIAStatus("READY"); err != nil {
		return fmt.Errorf("post-install NVIDIA status check failed: %v", err)
	}

	// Test NVIDIA + process
	if err := testCombinedWorkloads(); err != nil {
		return fmt.Errorf("combined workload test failed: %v", err)
	}

	return nil
}

func maskNVIDIASmi() error {
	cmd := exec.Command("sudo", "mv", "/usr/bin/nvidia-smi", "/usr/bin/nvidia-smi.backup")
	return cmd.Run()
}

func unmaskNVIDIASmi() error {
	cmd := exec.Command("sudo", "mv", "/usr/bin/nvidia-smi.backup", "/usr/bin/nvidia-smi")
	return cmd.Run()
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
	manifestData := map[string]string{
		"Main-Class": "Main",
	}
	if err := util.CreateTestJAR("/tmp/simple-test.jar", manifestData); err != nil {
		return fmt.Errorf("failed to create simple JAR: %v", err)
	}

	jvmCmd := exec.Command("java", "-cp", "/tmp", "-Dcom.sun.management.jmxremote.port=2030",
		"-Dcom.sun.management.jmxremote.authenticate=false",
		"-Dcom.sun.management.jmxremote.ssl=false",
		"-jar", "/tmp/simple-test.jar")

	if err := jvmCmd.Start(); err != nil {
		return fmt.Errorf("failed to start JVM: %v", err)
	}

	log.Printf("Started simple JVM process with PID: %d", jvmCmd.Process.Pid)
	time.Sleep(util.Sleep)

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
