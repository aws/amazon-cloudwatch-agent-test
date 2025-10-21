// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package unix

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/unix/util"
)

const JVMPort = 2030

func RunJVMTest() error {
	testCases := []struct {
		name         string
		jarName      string
		manifestData map[string]string
		expectedName string
	}{
		{
			name:    "Application-Name",
			jarName: "app-name-test.jar",
			manifestData: map[string]string{
				"Application-Name":     "My Application",
				"Implementation-Title": "Implementation Title",
				"Start-Class":          "com.example.Application",
				"Main-Class":           "Main",
			},
			expectedName: "My Application",
		},
		{
			name:    "Implementation-Title",
			jarName: "impl-title-test.jar",
			manifestData: map[string]string{
				"Implementation-Title": "Implementation Title",
				"Start-Class":          "com.example.Application",
				"Main-Class":           "Main",
			},
			expectedName: "Implementation Title",
		},
		{
			name:    "Start-Class",
			jarName: "start-class-test.jar",
			manifestData: map[string]string{
				"Start-Class": "com.example.Application",
				"Main-Class":  "Main",
			},
			expectedName: "com.example.Application",
		},
		{
			name:    "Main-Class",
			jarName: "main-class-test.jar",
			manifestData: map[string]string{
				"Main-Class": "Main",
			},
			expectedName: "Main",
		},
	}

	var errors []string
	for _, tc := range testCases {
		log.Printf("Testing JVM workload detection: %s", tc.name)

		if err := testJVMCase(tc.jarName, tc.manifestData, tc.expectedName); err != nil {
			errors = append(errors, fmt.Sprintf("JVM test case %s failed: %v", tc.name, err))
		} else {
			log.Printf("JVM test case %s completed successfully", tc.name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("JVM test failures: %s", strings.Join(errors, "; "))
	}

	return nil
}

func testJVMCase(jarName string, manifestData map[string]string, expectedName string) error {
	// Setup JAR
	jarPath := filepath.Join("/tmp", jarName)
	if err := util.CreateTestJAR(jarPath, manifestData); err != nil {
		return fmt.Errorf("failed to create test JAR: %v", err)
	}
	defer os.Remove(jarPath)

	// Test NEEDS_SETUP phase
	cmd := exec.Command("./unix/util/scripts", "spin_up_jvm", jarPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to start JVM without port: %v", err)
	}
	pidNoJMX := strings.TrimSpace(string(output))
	log.Printf("Started JVM process without port, PID: %s", pidNoJMX)

	time.Sleep(util.MediumSleep)
	if err := util.CheckJavaStatusWithRetry("NEEDS_SETUP/JMX_PORT", expectedName, "JVM", JVMPort); err != nil {
		exec.Command("./unix/util/scripts", "tear_down_jvm", pidNoJMX).Run()
		return fmt.Errorf("JVM NEEDS_SETUP status check failed: %v", err)
	}
	exec.Command("./unix/util/scripts", "tear_down_jvm", pidNoJMX).Run()

	// Test READY phase
	cmd = exec.Command("./unix/util/scripts", "spin_up_jvm", jarPath, strconv.Itoa(JVMPort))
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to start JVM with port: %v", err)
	}
	pidJMX := strings.TrimSpace(string(output))
	log.Printf("Started JVM process with port, PID: %s", pidJMX)

	time.Sleep(util.MediumSleep)
	if err := util.CheckJavaStatusWithRetry("READY", expectedName, "JVM", JVMPort); err != nil {
		exec.Command("./unix/util/scripts", "tear_down_jvm", pidJMX).Run()
		return fmt.Errorf("JVM READY status check failed: %v", err)
	}
	exec.Command("./unix/util/scripts", "tear_down_jvm", pidJMX).Run()
	time.Sleep(util.LongSleep)

	return nil
}
