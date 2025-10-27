// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package workload_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	jvmPort    = 2030
	kafkaPort  = 9999
	tomcatPort = 1080
)

var env *environment.MetaData

func Validate() error {
	env = environment.GetEnvironmentMetaData()

	platform := getPlatform()
	if err := platform.installJava17(); err != nil {
		fmt.Printf("Java 17 installation failed: %s\n", err.Error())
	}

	instanceType := awsservice.GetInstanceType()
	var errors []string

	if strings.HasPrefix(instanceType, "g4dn") {
		if err := runNVIDIA(platform); err != nil {
			errors = append(errors, "NVIDIA test failed: "+err.Error())
		}
	} else {
		if err := verifyWorkloadsEmpty(platform.agentCtlCmd()); err != nil {
			errors = append(errors, "Initial workloads not empty: "+err.Error())
		}
		if err := runJVM(platform); err != nil {
			errors = append(errors, "JVM test failed: "+err.Error())
		}
		if err := runTomcat(platform); err != nil {
			errors = append(errors, "Tomcat test failed: "+err.Error())
		}
		if err := runKafka(platform); err != nil {
			errors = append(errors, "Kafka test failed: "+err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("workload discovery validation failed: %s", strings.Join(errors, "; "))
	}
	return nil
}

// Processes
//
// testWorkload function signature:
//   testWorkload(p platform, label, identifier, expectedName, workloadType, needsSetupStatus string, port int,
//     setupFn func() error,
//     startNeedsSetup func() ([]string, func(string) []string),
//     startReady func() ([]string, func(string) []string)) error
//
// The three function parameters control the test workflow:
//   1. setupFn: Prepares resources (e.g., create JAR, download/extract archives)
//   2. startNeedsSetup: Returns (startCmd, stopCmd) to start process WITHOUT complete setup (e.g. missing telemetry port)
//      - startCmd: Command array to start the process
//      - stopCmd: Function that takes PID and returns command array to stop the process
//      - After execution, checks for NEEDS_SETUP status (jmxSetupStatus or nvidiaSetupStatus)
//   3. startReady: Returns (startCmd, stopCmd) to start process WITH complete setup
//      - After execution, checks for READY status

func runJVM(p platform) error {
	testCases := []struct {
		jarName      string
		manifestData map[string]string
		expectedName string
	}{
		{"app-name-test.jar", map[string]string{"Application-Name": "My Application", "Implementation-Title": "Implementation Title", "Start-Class": "com.example.Application", "Main-Class": "Main"}, "My Application"},
		{"impl-title-test.jar", map[string]string{"Implementation-Title": "Implementation Title", "Start-Class": "com.example.Application", "Main-Class": "Main"}, "Implementation Title"},
		{"start-class-test.jar", map[string]string{"Start-Class": "com.example.Application", "Main-Class": "Main"}, "com.example.Application"},
		{"main-class-test.jar", map[string]string{"Main-Class": "Main"}, "Main"},
	}

	for _, tc := range testCases {
		// Create directory for JAR file
		jarPath := filepath.Join(p.tmpDir(), tc.jarName)
		defer os.Remove(jarPath)

		if err := testWorkload(p, "JVM", tc.expectedName, tc.expectedName, "JVM", jmxSetupStatus, jvmPort,
			func() error { return p.createTestJAR(jarPath, tc.manifestData) },                           // Setup: Create JAR file
			func() ([]string, func(string) []string) { return p.startJVM(jarPath, 0), p.stopJVM },       // Start without JMX port → NEEDS_SETUP
			func() ([]string, func(string) []string) { return p.startJVM(jarPath, jvmPort), p.stopJVM }, // Start with JMX port → READY
		); err != nil {
			return err
		}
	}
	return nil
}

func runTomcat(p platform) error {
	for _, version := range []string{"apache-tomcat-9.0.110", "apache-tomcat-10.1.47", "apache-tomcat-11.0.12"} {
		// Create directory for Tomcat
		tomcatDir := filepath.Join(p.tmpDir(), "tomcat", version)
		defer os.RemoveAll(filepath.Join(p.tmpDir(), "tomcat"))

		if err := testWorkload(p, "Tomcat", version, "apache-tomcat", "TOMCAT", jmxSetupStatus, tomcatPort,
			func() error { return p.setupTomcat(version, env.Bucket) },                                               // Setup: Download and extract Tomcat
			func() ([]string, func(string) []string) { return p.startTomcat(tomcatDir, 0), p.stopTomcat(tomcatDir) }, // Start without JMX port → NEEDS_SETUP
			func() ([]string, func(string) []string) {
				return p.startTomcat(tomcatDir, tomcatPort), p.stopTomcat(tomcatDir)
			}, // Start with JMX port → READY
		); err != nil {
			return err
		}
	}
	return nil
}

func runKafka(p platform) error {
	for _, version := range []string{"kafka_2.13-3.5.0", "kafka_2.13-4.0.0"} {
		// Create directory for Kafka
		kafkaDir := filepath.Join(p.tmpDir(), "kafka", version)
		defer os.RemoveAll(filepath.Join(p.tmpDir(), "kafka"))

		if err := testWorkload(p, "Kafka", version, "Kafka Broker", "KAFKA/BROKER", jmxSetupStatus, kafkaPort,
			func() error { return p.setupKafka(version, env.Bucket) }, // Setup: Download and extract Kafka
			func() ([]string, func(string) []string) {
				return p.startKafka(kafkaDir, version, 0), p.stopKafka(kafkaDir, version)
			}, // Start without JMX port → NEEDS_SETUP
			func() ([]string, func(string) []string) {
				return p.startKafka(kafkaDir, version, kafkaPort), p.stopKafka(kafkaDir, version)
			}, // Start with JMX port → READY
		); err != nil {
			return err
		}
	}
	return nil
}

// Devices
//
// Device-based workloads (like NVIDIA GPU) differ from process-based workloads:
// - They don't have start/stop lifecycle - the device is always present once installed
// - We use dummy functions (returning nil) to trigger status checks without starting processes
// - NVIDIA workflow: setupNVIDIADevice → check NEEDS_SETUP → installNVIDIADriver → check READY

func runNVIDIA(p platform) error {
	// Set up NVIDIA device if needed
	if err := p.setupNVIDIADevice(); err != nil {
		return err
	}
	defer p.uninstallNVIDIA()

	// Test NVIDIA GPU detection: NEEDS_SETUP → READY transition
	if err := testWorkload(p, "NVIDIA", "device", "", "NVIDIA_GPU", nvidiaSetupStatus, 0,
		nil, // No setup needed (device already created)
		func() ([]string, func(string) []string) { return nil, nil },                               // Dummy function: just sleep and check NEEDS_SETUP status
		func() ([]string, func(string) []string) { return p.installNVIDIADriver(env.Bucket), nil }, // Install driver (no stop needed) → READY
	); err != nil {
		return err
	}

	fmt.Println("[NVIDIA] Testing combined workload detection (GPU + JVM)")
	// Create directory for JAR file
	jarPath := filepath.Join(p.tmpDir(), "simple-test.jar")
	defer os.Remove(jarPath)

	if err := testWorkload(p, "JVM", "Main", "Main", "JVM", jmxSetupStatus, 0,
		func() error { return p.createTestJAR(jarPath, map[string]string{"Main-Class": "Main"}) }, // Setup: Create JAR
		nil, // Skip NEEDS_SETUP check for JVM in this test
		func() ([]string, func(string) []string) { return p.startJVM(jarPath, jvmPort), p.stopJVM }, // Start JVM with port → READY
	); err != nil {
		return err
	}

	// Verify GPU is still READY after JVM had started
	return checkStatus(p.agentCtlCmd(), ready, "", "NVIDIA_GPU", 0)
}
