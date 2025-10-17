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
	// Set up JAR
	jarPath := filepath.Join("/tmp", jarName)
	if err := util.CreateTestJAR(jarPath, manifestData); err != nil {
		return fmt.Errorf("failed to create test JAR: %v", err)
	}
	defer os.Remove(jarPath)

	// Create JVM process with no JMX to test NEEDS_SETUP phase
	jvmCmdNoJMX := exec.Command("java", "-jar", jarPath)
	log.Printf("Starting JVM without JMX configured: %s", jarName)
	if err := jvmCmdNoJMX.Start(); err != nil {
		return fmt.Errorf("failed to start JVM without JMX configured: %v", err)
	}
	log.Printf("Started JVM process without JMX, PID: %d", jvmCmdNoJMX.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("NEEDS_SETUP/JMX_PORT", expectedName, "JVM", JVMPort); err != nil {
		jvmCmdNoJMX.Process.Kill()
		return fmt.Errorf("JVM NEEDS_SETUP status check failed: %v", err)
	}
	jvmCmdNoJMX.Process.Kill()
	time.Sleep(util.Sleep)

	// Create JVM process with JMX to test READY phase
	jvmCmd := exec.Command("java",
		"-Dcom.sun.management.jmxremote",
		fmt.Sprintf("-Dcom.sun.management.jmxremote.port=%d", JVMPort),
		"-Dcom.sun.management.jmxremote.local.only=false",
		"-Dcom.sun.management.jmxremote.authenticate=false",
		"-Dcom.sun.management.jmxremote.ssl=false",
		fmt.Sprintf("-Dcom.sun.management.jmxremote.rmi.port=%d", JVMPort),
		"-Dcom.sun.management.jmxremote.host=localhost",
		"-Djava.rmi.server.hostname=localhost",
		"-jar", jarPath)
	log.Printf("Starting JVM with JMX configured: %s", jarName)
	if err := jvmCmd.Start(); err != nil {
		return fmt.Errorf("failed to start JVM with JMX: %v", err)
	}
	log.Printf("Started JVM process with JMX, PID: %d", jvmCmd.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("READY", expectedName, "JVM", JVMPort); err != nil {
		jvmCmd.Process.Kill()
		return fmt.Errorf("JVM READY status check failed: %v", err)
	}
	jvmCmd.Process.Kill()
	time.Sleep(util.Sleep)

	return nil
}
