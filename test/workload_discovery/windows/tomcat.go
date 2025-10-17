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
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
)

const TomcatPort = 1080

func RunTomcatTest() error {
	versions := []string{
		"apache-tomcat-9.0.110.tar.gz",
		"apache-tomcat-10.1.47.tar.gz",
		"apache-tomcat-11.0.12.tar.gz",
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
	// Set up Tomcat environment
	if err := util.SetupJavaWorkload(version, "tomcat"); err != nil {
		return fmt.Errorf("failed to setup Tomcat %s: %v", version, err)
	}
	tomcatDir := fmt.Sprintf("C:\\temp\\%s", version[:len(version)-7])
	os.Setenv("CATALINA_HOME", tomcatDir)
	os.Setenv("CATALINA_BASE", tomcatDir)

	// Test NEEDS_SETUP phase
	tomcatCmdNoJMX := exec.Command(tomcatDir + "\\bin\\startup.bat")
	tomcatCmdNoJMX.Env = append(os.Environ(),
		"CATALINA_HOME="+tomcatDir,
		"CATALINA_BASE="+tomcatDir)
	if err := tomcatCmdNoJMX.Start(); err != nil {
		return fmt.Errorf("failed to start Tomcat without JMX: %v", err)
	}
	log.Printf("Started Tomcat process (no JMX) with PID: %d", tomcatCmdNoJMX.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("NEEDS_SETUP/JMX_PORT", "apache-tomcat", "TOMCAT", TomcatPort); err != nil {
		return fmt.Errorf("initial Tomcat status check failed for %s: %v", version, err)
	}
	exec.Command(tomcatDir + "\\bin\\shutdown.bat").Run()
	tomcatCmdNoJMX.Process.Kill()
	time.Sleep(util.Sleep)
	exec.Command("taskkill", "/F", "/IM", "java.exe").Run()

	// Test READY phase
	os.Setenv("CATALINA_OPTS", fmt.Sprintf("-Dcom.sun.management.jmxremote.port=%d -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false", TomcatPort))
	tomcatCmd := exec.Command(tomcatDir + "\\bin\\startup.bat")
	tomcatCmd.Env = append(os.Environ(),
		"CATALINA_HOME="+tomcatDir,
		"CATALINA_BASE="+tomcatDir,
		fmt.Sprintf("CATALINA_OPTS=-Dcom.sun.management.jmxremote.port=%d -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false", TomcatPort))
	if err := tomcatCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Tomcat with JMX: %v", err)
	}
	log.Printf("Started Tomcat process with PID: %d", tomcatCmd.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("READY", "apache-tomcat", "TOMCAT", TomcatPort); err != nil {
		return fmt.Errorf("post-start Tomcat status check failed for %s: %v", version, err)
	}
	exec.Command(tomcatDir + "\\bin\\shutdown.bat").Run()
	tomcatCmd.Process.Kill()
	time.Sleep(util.Sleep)
	exec.Command("taskkill", "/F", "/IM", "java.exe").Run()
	exec.Command("taskkill", "/F", "/IM", "javac.exe").Run()
	exec.Command("taskkill", "/F", "/T", "/IM", "java.exe").Run()
	exec.Command("powershell", "-Command", "Remove-Item -Recurse -Force '"+tomcatDir+"\\logs' -ErrorAction SilentlyContinue").Run()
	exec.Command("powershell", "-Command", "Remove-Item -Recurse -Force '"+tomcatDir+"\\work' -ErrorAction SilentlyContinue").Run()
	exec.Command("powershell", "-Command", "Remove-Item -Recurse -Force '"+tomcatDir+"\\temp' -ErrorAction SilentlyContinue").Run()
	os.Unsetenv("CATALINA_OPTS")
	os.Unsetenv("CATALINA_BASE")
	os.Unsetenv("CATALINA_HOME")

	return nil
}
