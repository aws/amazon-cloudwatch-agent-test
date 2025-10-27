// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package workload_discovery

type platform interface {
	tmpDir() string
	agentCtlCmd() []string
	installJava17() error
	createTestJAR(path string, manifestData map[string]string) error

	startJVM(jarPath string, port int) []string
	stopJVM(pid string) []string

	setupKafka(version, bucket string) error
	startKafka(kafkaDir, version string, port int) []string
	stopKafka(kafkaDir, version string) func(string) []string

	setupTomcat(version, bucket string) error
	startTomcat(tomcatDir string, port int) []string
	stopTomcat(tomcatDir string) func(string) []string

	setupNVIDIADevice() error
	installNVIDIADriver(bucket string) []string
	uninstallNVIDIA() error
}
