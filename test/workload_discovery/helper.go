// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package workload_discovery

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	workloadUptimeSleep = 10 * time.Second // Uptime set in workload discovery binary (https://github.com/aws/amazon-cloudwatch-agent/blob/c92d18fd394be0e487e3e847df65eda468dcd340/cmd/workload-discovery/discovery.go#L227)
	processSleep        = 12 * time.Second // Wait for process to start/end
	jmxSetupStatus      = "NEEDS_SETUP/JMX_PORT"
	nvidiaSetupStatus   = "NEEDS_SETUP/NVIDIA_DRIVER"
	ready               = "READY"
)

type workloadStatus struct {
	Status    string     `json:"status"`
	StartTime string     `json:"starttime"`
	Config    string     `json:"configstatus"`
	Version   string     `json:"version"`
	Workloads []workload `json:"workloads"`
}

type workload struct {
	Categories    []string `json:"categories"`
	Name          string   `json:"name"`
	TelemetryPort int      `json:"telemetry_port,omitempty"`
	Status        string   `json:"status"`
}

func getWorkloads(agentCtlCmd []string) ([]workload, error) {
	cmd := exec.Command(agentCtlCmd[0], agentCtlCmd[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload status: %v", err)
	}
	fmt.Printf("CloudWatch Agent status output: %s\n", string(output))

	var status workloadStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse workloads status: %v", err)
	}
	return status.Workloads, nil
}

func verifyWorkloadsEmpty(agentCtlCmd []string) error {
	workloads, err := getWorkloads(agentCtlCmd)
	if err != nil {
		return err
	}
	if len(workloads) != 0 {
		return fmt.Errorf("workloads are not empty")
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func checkStatus(agentCtlCmd []string, expectedStatus, expectedName, workloadType string, port int) error {
	workloads, err := getWorkloads(agentCtlCmd)
	if err != nil {
		return err
	}

	for _, w := range workloads {
		nameMatch := expectedName == "" || strings.Contains(w.Name, expectedName)
		categoryMatch := workloadType == "" || contains(w.Categories, workloadType)

		if nameMatch && categoryMatch && w.Status == expectedStatus {
			if expectedStatus == ready && port > 0 && w.TelemetryPort != port {
				return fmt.Errorf("%s workload has wrong telemetry port, expected %d, got %d", workloadType, port, w.TelemetryPort)
			}
			return nil
		}
	}
	return fmt.Errorf("workload %s with status %s not found", workloadType, expectedStatus)
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func testWorkload(p platform, label, identifier, expectedName, workloadType, needsSetupStatus string, port int,
	setupFn func() error,
	startNeedsSetup func() ([]string, func(string) []string),
	startReady func() ([]string, func(string) []string)) error {

	fmt.Printf("[%s] Testing: %s\n", label, identifier)

	if setupFn != nil {
		fmt.Printf("[%s] SETUP phase for %s\n", label, identifier)
		if err := setupFn(); err != nil {
			return err
		}
	}

	if startNeedsSetup != nil {
		fmt.Printf("[%s] NEEDS_SETUP phase for %s\n", label, identifier)
		startCmd, stopCmd := startNeedsSetup()
		var needsSetupPid string
		if startCmd != nil {
			pid, err := runCommand(startCmd[0], startCmd[1:]...)
			if err != nil {
				return err
			}
			needsSetupPid = pid
		}
		time.Sleep(workloadUptimeSleep)
		if err := checkStatus(p.agentCtlCmd(), needsSetupStatus, expectedName, workloadType, port); err != nil {
			return err
		}
		if needsSetupPid != "" && stopCmd != nil {
			stop := stopCmd(needsSetupPid)
			runCommand(stop[0], stop[1:]...)
		}
	}

	time.Sleep(processSleep)

	if startReady != nil {
		fmt.Printf("[%s] READY phase for %s\n", label, identifier)
		startCmd, stopCmd := startReady()
		if startCmd != nil {
			pid, err := runCommand(startCmd[0], startCmd[1:]...)
			if err != nil {
				return err
			}
			if stopCmd != nil {
				defer func() {
					stop := stopCmd(pid)
					runCommand(stop[0], stop[1:]...)
				}()
			}
		}
		time.Sleep(workloadUptimeSleep)
		return checkStatus(p.agentCtlCmd(), ready, expectedName, workloadType, port)
	}

	time.Sleep(processSleep)

	return nil
}
