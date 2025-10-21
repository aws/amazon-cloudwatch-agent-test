// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package util

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	ShortSleep  = 2 * time.Second
	MediumSleep = 5 * time.Second
	LongSleep   = 10 * time.Second
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

func GetWorkloads() ([]Workload, error) {
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "status-with-workloads")
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

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func CheckJavaStatus(expectedStatus string, expectedName string, workloadType string, port int) error {
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

func CheckJavaStatusWithRetry(expectedStatus string, expectedName string, workloadType string, port int) error {
	var lastErr error
	for i := 0; i < 3; i++ {
		if err := CheckJavaStatus(expectedStatus, expectedName, workloadType, port); err == nil {
			return nil
		} else {
			lastErr = err
			log.Printf("CheckJavaStatus attempt %d failed: %v", i+1, err)
			if i < 2 {
				time.Sleep(LongSleep)
			}
		}
	}
	return fmt.Errorf("CheckJavaStatus failed after 3 attempts: %v", lastErr)
}

func InstallJava17() error {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("./unix/util/scripts", "install_java17", env.Bucket)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("InstallJava17 failed: %v, output: %s", err, string(output))
	}
	return nil
}

func CreateTestJAR(path string, manifestData map[string]string) error {
	jarName := strings.TrimPrefix(path, "/tmp/")

	args := []string{"setup_jar", jarName}
	for k, v := range manifestData {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.Command("./unix/util/scripts", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("CreateTestJAR failed: %v, output: %s", err, string(output))
	}
	return nil
}

func SetupJavaWorkload(version string, workloadType string) error {
	var setupFunc string
	switch workloadType {
	case "tomcat":
		setupFunc = "setup_tomcat"
	case "kafka":
		setupFunc = "setup_kafka"
	default:
		return fmt.Errorf("unknown workload type: %s", workloadType)
	}

	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("./unix/util/scripts", setupFunc, version, env.Bucket)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SetupJavaWorkload failed: %v, output: %s", err, string(output))
	}
	return nil
}
