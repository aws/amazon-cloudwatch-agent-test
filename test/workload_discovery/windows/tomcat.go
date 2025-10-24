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

const TomcatPort = 1080

func RunTomcatTest() error {
	versions := []string{
		"apache-tomcat-9.0.110",
		"apache-tomcat-10.1.47",
		"apache-tomcat-11.0.12",
	}

	var errors []string
	for _, version := range versions {
		log.Printf("Testing Tomcat version: %s", version)

		if err := testTomcatVersion(version); err != nil {
			errors = append(errors, fmt.Sprintf("Tomcat %s test failed: %v", version, err))
		} else {
			log.Printf("Tomcat version %s test completed successfully", version)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("tomcat test failures: %s", strings.Join(errors, "; "))
	}

	return nil
}

func testTomcatVersion(version string) error {
	defer func() {
		time.Sleep(util.RaceConditionSleep)
		os.RemoveAll("C:\\tmp\\tomcat")
	}()
	// Setup Tomcat
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Setup-Tomcat", "-Version", version, "-Bucket", env.Bucket)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to setup Tomcat %s: %v", version, err)
	}
	tomcatDir := fmt.Sprintf("C:\\tmp\\tomcat\\%s", version)
	time.Sleep(util.RaceConditionSleep)

	// Test NEEDS_SETUP phase
	cmd = exec.Command("powershell", "-File", "C:\\scripts.ps1", "Start-Tomcat", "-TomcatDir", tomcatDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Tomcat without JMX: %v", err)
	}
	time.Sleep(util.WorkloadUptimeSleep)
	if err := util.CheckStatusWithRetry(util.JMXSetupStatus, "apache-tomcat", "TOMCAT", TomcatPort); err != nil {
		exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Tomcat", "-TomcatDir", tomcatDir).Run()
		return fmt.Errorf("initial Tomcat status check failed for %s: %v", version, err)
	}
	exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Tomcat", "-TomcatDir", tomcatDir).Run()
	time.Sleep(util.RaceConditionSleep)

	// Test READY phase
	cmd = exec.Command("powershell", "-File", "C:\\scripts.ps1", "Start-Tomcat", "-TomcatDir", tomcatDir, "-Port", strconv.Itoa(TomcatPort))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Tomcat with JMX: %v", err)
	}
	time.Sleep(util.WorkloadUptimeSleep)
	if err := util.CheckStatusWithRetry(util.Ready, "apache-tomcat", "TOMCAT", TomcatPort); err != nil {
		exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Tomcat", "-TomcatDir", tomcatDir).Run()
		return fmt.Errorf("post-start Tomcat status check failed for %s: %v", version, err)
	}
	exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Tomcat", "-TomcatDir", tomcatDir).Run()
	time.Sleep(util.RaceConditionSleep)

	return nil
}
