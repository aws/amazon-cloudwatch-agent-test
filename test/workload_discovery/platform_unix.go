// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package workload_discovery

import (
	"fmt"
	"strconv"
	"strings"
)

type unixPlatform struct{}

func getPlatform() platform { return &unixPlatform{} }

func (p *unixPlatform) tmpDir() string { return "/tmp" }

func (p *unixPlatform) agentCtlCmd() []string {
	return []string{"sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "status-with-workloads"}
}

func (p *unixPlatform) installJava17() error {
	_, err := runCommand("./scripts_unix.sh", "install_java17", env.Bucket)
	return err
}

func (p *unixPlatform) createTestJAR(path string, manifestData map[string]string) error {
	jarName := strings.TrimPrefix(path, "/tmp/")
	args := []string{"./scripts_unix.sh", "setup_jar", jarName}
	for k, v := range manifestData {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	_, err := runCommand(args[0], args[1:]...)
	return err
}

func (p *unixPlatform) startJVM(jarPath string, port int) []string {
	if port == 0 {
		return []string{"./scripts_unix.sh", "spin_up_jvm", jarPath}
	}
	return []string{"./scripts_unix.sh", "spin_up_jvm", jarPath, strconv.Itoa(port)}
}

func (p *unixPlatform) stopJVM(pid string) []string {
	return []string{"./scripts_unix.sh", "tear_down_jvm", pid}
}

func (p *unixPlatform) setupKafka(version, bucket string) error {
	_, err := runCommand("./scripts_unix.sh", "setup_kafka", version, bucket)
	return err
}

func (p *unixPlatform) startKafka(kafkaDir, version string, port int) []string {
	if port == 0 {
		return []string{"./scripts_unix.sh", "spin_up_kafka", kafkaDir, version}
	}
	return []string{"./scripts_unix.sh", "spin_up_kafka", kafkaDir, version, strconv.Itoa(port)}
}

func (p *unixPlatform) stopKafka(kafkaDir, version string) func(string) []string {
	return func(pid string) []string {
		return []string{"./scripts_unix.sh", "tear_down_kafka", pid, kafkaDir, version}
	}
}

func (p *unixPlatform) setupTomcat(version, bucket string) error {
	_, err := runCommand("./scripts_unix.sh", "setup_tomcat", version, bucket)
	return err
}

func (p *unixPlatform) startTomcat(tomcatDir string, port int) []string {
	if port == 0 {
		return []string{"./scripts_unix.sh", "spin_up_tomcat", tomcatDir}
	}
	return []string{"./scripts_unix.sh", "spin_up_tomcat", tomcatDir, strconv.Itoa(port)}
}

func (p *unixPlatform) stopTomcat(tomcatDir string) func(string) []string {
	return func(pid string) []string {
		return []string{"./scripts_unix.sh", "tear_down_tomcat", pid, tomcatDir}
	}
}

func (p *unixPlatform) setupNVIDIADevice() error {
	_, err := runCommand("./scripts_unix.sh", "setup_nvidia_device")
	return err
}

func (p *unixPlatform) installNVIDIADriver(bucket string) []string {
	return []string{"./scripts_unix.sh", "install_nvidia_driver", bucket}
}

func (p *unixPlatform) uninstallNVIDIA() error {
	_, err := runCommand("./scripts_unix.sh", "uninstall_nvidia")
	return err
}
