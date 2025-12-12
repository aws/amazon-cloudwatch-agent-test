// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package workload_discovery

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

//go:embed scripts_windows.ps1
var scriptsPS1 string

type windowsPlatform struct{}

func getPlatform() platform {
	writeEmbeddedScript()
	return &windowsPlatform{}
}

func (p *windowsPlatform) tmpDir() string { return "C:\\tmp" }

func (p *windowsPlatform) agentCtlCmd() []string {
	return []string{"powershell", "-File", "C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1", "-a", "status-with-workloads"}
}

func (p *windowsPlatform) installJava17() error {
	_, err := runCommand("powershell", "-File", "C:\\scripts.ps1", "Install-Java17", env.Bucket)
	return err
}

func (p *windowsPlatform) createTestJAR(path string, manifestData map[string]string) error {
	jarName := filepath.Base(path)
	args := []string{"powershell", "-File", "C:\\scripts.ps1", "Setup-Jar", jarName}
	for k, v := range manifestData {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	_, err := runCommand(args[0], args[1:]...)
	return err
}

func (p *windowsPlatform) startJVM(jarPath string, port int) []string {
	if port == 0 {
		return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-JVM", "-JarPath", jarPath}
	}
	return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-JVM", "-JarPath", jarPath, "-Port", strconv.Itoa(port)}
}

func (p *windowsPlatform) stopJVM(pid string) []string {
	return []string{"powershell", "-File", "C:\\scripts.ps1", "Stop-JVM", "-ProcessId", pid}
}

func (p *windowsPlatform) setupKafka(version, bucket string) error {
	_, err := runCommand("powershell", "-File", "C:\\scripts.ps1", "Setup-Kafka", "-Version", version, "-Bucket", bucket)
	return err
}

func (p *windowsPlatform) startKafka(kafkaDir, version string, port int) []string {
	if port == 0 {
		return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-Kafka", "-KafkaDir", kafkaDir, "-Version", version}
	}
	return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-Kafka", "-KafkaDir", kafkaDir, "-Version", version, "-Port", strconv.Itoa(port)}
}

func (p *windowsPlatform) stopKafka(kafkaDir, version string) func(string) []string {
	return func(pid string) []string {
		return []string{"powershell", "-File", "C:\\scripts.ps1", "Stop-Kafka", "-ProcessId", pid, "-KafkaDir", kafkaDir, "-Version", version}
	}
}

func (p *windowsPlatform) setupTomcat(version, bucket string) error {
	_, err := runCommand("powershell", "-File", "C:\\scripts.ps1", "Setup-Tomcat", "-Version", version, "-Bucket", bucket)
	return err
}

func (p *windowsPlatform) startTomcat(tomcatDir string, port int) []string {
	if port == 0 {
		return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-Tomcat", "-TomcatDir", tomcatDir}
	}
	return []string{"powershell", "-File", "C:\\scripts.ps1", "Start-Tomcat", "-TomcatDir", tomcatDir, "-Port", strconv.Itoa(port)}
}

func (p *windowsPlatform) stopTomcat(tomcatDir string) func(string) []string {
	return func(pid string) []string {
		return []string{"powershell", "-File", "C:\\scripts.ps1", "Stop-Tomcat", "-ProcessId", pid, "-TomcatDir", tomcatDir}
	}
}

func (p *windowsPlatform) setupNVIDIADevice() error {
	return nil // Windows doesn't need setup
}

func (p *windowsPlatform) installNVIDIADriver(bucket string) []string {
	return []string{"powershell", "-File", "C:\\scripts.ps1", "Install-NvidiaDriver", "-Bucket", bucket}
}

func (p *windowsPlatform) uninstallNVIDIA() error {
	_, err := runCommand("powershell", "-File", "C:\\scripts.ps1", "Uninstall-NvidiaDriver")
	return err
}

func writeEmbeddedScript() error {
	return os.WriteFile("C:\\scripts.ps1", []byte(scriptsPS1), 0644)
}
