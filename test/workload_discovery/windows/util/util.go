// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package util

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

//go:embed scripts.ps1
var scriptsPS1 string

const (
	RaceConditionSleep  = 5 * time.Second
	WorkloadUptimeSleep = 10 * time.Second
	JMXSetupStatus      = "NEEDS_SETUP/JMX_PORT"
	NVIDIASetupStatus   = "NEEDS_SETUP/NVIDIA_DRIVER"
	Ready               = "READY"
)

type WorkloadStatus struct {
	Status    string     `json:"status"`
	StartTime string     `json:"starttime"`
	Config    string     `json:"configstatus"`
	Version   string     `json:"version"`
	Workloads []Workload `json:"workloads"`
}

type Workload struct {
	Categories    []string `json:"categories"`
	Name          string   `json:"name"`
	TelemetryPort int      `json:"telemetry_port,omitempty"`
	Status        string   `json:"status"`
}

func InstallJava17() error {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Install-Java17", env.Bucket)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("InstallJava17 failed: %v, output: %s", err, string(output))
	}
	return nil
}

func GetWorkloads() ([]Workload, error) {
	cmd := exec.Command("powershell", "-File", "C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1", "-a", "status-with-workloads")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload status: %v", err)
	}
	fmt.Printf("CloudWatch Agent status output: %s\n", string(output))

	var status WorkloadStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse workloads status: %v", err)
	}

	return status.Workloads, nil
}

func VerifyWorkloadsEmpty() error {
	workloads, err := GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	if len(workloads) != 0 {
		return fmt.Errorf("workloads are not empty")
	}

	return nil
}

func CreateTestJAR(path string, manifestData map[string]string) error {
	jarName := filepath.Base(path)

	var manifestArgs []string
	for k, v := range manifestData {
		manifestArgs = append(manifestArgs, fmt.Sprintf("%s=%s", k, v))
	}

	args := []string{"powershell", "-File", "C:\\scripts.ps1", "Setup-Jar", jarName}
	args = append(args, manifestArgs...)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("CreateTestJAR failed: %v, output: %s", err, string(output))
	}
	return nil
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func CheckStatus(expectedStatus string, expectedName string, workloadType string, port int) error {
	workloads, err := GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	for _, workload := range workloads {
		if strings.Contains(workload.Name, expectedName) && workload.Status == expectedStatus {
			if expectedStatus == "READY" {
				if workload.TelemetryPort != port {
					return fmt.Errorf("%s workload has wrong telemetry port, expected %d, got %d", workloadType, port, workload.TelemetryPort)
				}
				if !Contains(workload.Categories, workloadType) {
					return fmt.Errorf("missing %s category, got: %v", workloadType, workload.Categories)
				}
			}
			return nil
		}
	}

	return fmt.Errorf("workload %s with status %s not found", expectedName, expectedStatus)
}

func CheckStatusWithRetry(expectedStatus string, expectedName string, workloadType string, port int) error {
	var lastErr error
	for i := 0; i < 3; i++ {
		if err := CheckStatus(expectedStatus, expectedName, workloadType, port); err == nil {
			return nil
		} else {
			lastErr = err
			log.Printf("CheckStatus attempt %d failed: %v", i+1, err)
			if i < 2 {
				time.Sleep(RaceConditionSleep)
			}
		}
	}
	return fmt.Errorf("CheckStatus failed after 3 attempts: %v", lastErr)
}

func WriteEmbeddedScript() error {
	return os.WriteFile("C:\\scripts.ps1", []byte(scriptsPS1), 0644)
}
