// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package windows

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func RunNVIDIATest() error {
	// Test NEEDS_SETUP phase
	if err := checkNVIDIAStatus("NEEDS_SETUP/NVIDIA_DRIVER"); err != nil {
		return fmt.Errorf("initial NVIDIA status check failed: %v", err)
	}

	// Test READY phase
	if err := installNVIDIADriver(); err != nil {
		return fmt.Errorf("failed to install NVIDIA driver: %v", err)
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

func installNVIDIADriver() error {
	var s3Key string
	if util.IsWindows2016() || util.IsWindows2019() {
		s3Key = "nvidia/windows/grid-9.1/431.79_grid_win10_server2016_server2019_64bit_international.exe"
	} else {
		s3Key = "nvidia/windows/latest/581.42_grid_win10_win11_server2019_server2022_server2025_dch_64bit_international_aws_swl.exe"
	}

	localPath := "C:\\temp\\nvidia-driver.exe"
	env := environment.GetEnvironmentMetaData()
	if err := awsservice.DownloadFile(env.Bucket, s3Key, localPath); err != nil {
		return err
	}

	psScript := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '/s', '/noreboot' -Wait -PassThru", localPath)
	cmd := exec.Command("powershell", "-Command", psScript)
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

	if err := util.CreateTestJAR("C:\\temp\\simple-test.jar", manifestData); err != nil {
		return fmt.Errorf("failed to create simple JAR: %v", err)
	}

	jvmCmd := exec.Command("java", "-cp", "C:\\temp", "-Dcom.sun.management.jmxremote.port=2030",
		"-Dcom.sun.management.jmxremote.authenticate=false",
		"-Dcom.sun.management.jmxremote.ssl=false",
		"-jar", "C:\\temp\\simple-test.jar")

	if err := jvmCmd.Start(); err != nil {
		return fmt.Errorf("failed to start JVM: %v", err)
	}

	defer jvmCmd.Process.Kill()

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
