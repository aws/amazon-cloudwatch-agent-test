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

const KafkaPort = 9999

func RunKafkaTest() error {
	versions := []string{"kafka_2.13-3.5.0.tgz", "kafka_2.13-4.0.0.tgz"}

	var errors []string
	for _, version := range versions {
		log.Printf("Testing Kafka version: %s", version)

		if err := testKafkaVersion(version); err != nil {
			errors = append(errors, fmt.Sprintf("Kafka %s test failed: %v", version, err))
		} else {
			log.Printf("Kafka version %s test completed successfully", version)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("kafka test failures: %s", strings.Join(errors, "; "))
	}

	return nil
}

func testKafkaVersion(version string) error {
	// Set up Kafka environment
	if err := util.SetupJavaWorkload(version, "kafka"); err != nil {
		return fmt.Errorf("failed to setup Kafka %s: %v", version, err)
	}
	kafkaDir := fmt.Sprintf("/tmp/%s", version[:len(version)-4])
	var zookeeperCmd *exec.Cmd
	var configFile string

	// Test NEEDS_SETUP phase
	if isKafkaVersionUnder4(version) {
		zookeeperCmd = exec.Command(filepath.Join(kafkaDir, "bin/zookeeper-server-start.sh"),
			filepath.Join(kafkaDir, "config/zookeeper.properties"))
		zookeeperCmd.Start()
		time.Sleep(util.Sleep)
		configFile = filepath.Join(kafkaDir, "config/server.properties")
	} else {
		configFile = filepath.Join(kafkaDir, "config/controller.properties")
		uuidCmd := exec.Command(filepath.Join(kafkaDir, "bin/kafka-storage.sh"), "random-uuid")
		uuidCmd.Dir = kafkaDir
		uuidOutput, err := uuidCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to generate cluster UUID: %v, output: %s", err, string(uuidOutput))
		}
		clusterUUID := strings.TrimSpace(string(uuidOutput))
		exec.Command("rm", "-rf", "/tmp/kraft-controller-logs").Run()
		formatCmd := exec.Command(filepath.Join(kafkaDir, "bin/kafka-storage.sh"),
			"format", "-t", clusterUUID, "-c", configFile, "--standalone")
		if output, err := formatCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to format Kafka storage: %v, output: %s", err, string(output))
		}
	}
	kafkaCmdNoJMX := exec.Command(filepath.Join(kafkaDir, "bin/kafka-server-start.sh"), configFile)
	if err := kafkaCmdNoJMX.Start(); err != nil {
		return fmt.Errorf("failed to start Kafka with no JMX: %v", err)
	}
	log.Printf("Started Kafka process with PID: %d", kafkaCmdNoJMX.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("NEEDS_SETUP/JMX_PORT", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		return fmt.Errorf("initial Kafka status check failed for %s: %v", version, err)
	}
	exec.Command(filepath.Join(kafkaDir, "bin/kafka-server-stop.sh")).Run()
	if isKafkaVersionUnder4(version) {
		exec.Command(filepath.Join(kafkaDir, "bin/zookeeper-server-stop.sh")).Run()
	}
	exec.Command("rm", "-rf", "/tmp/zookeeper").Run()
	exec.Command("rm", "-rf", "/tmp/kafka-logs").Run()

	// Test READY phase
	if isKafkaVersionUnder4(version) {
		zookeeperCmd = exec.Command(filepath.Join(kafkaDir, "bin/zookeeper-server-start.sh"),
			filepath.Join(kafkaDir, "config/zookeeper.properties"))
		zookeeperCmd.Start()
		time.Sleep(util.Sleep)
	}
	os.Setenv("KAFKA_JMX_OPTS", fmt.Sprintf("-Dcom.sun.management.jmxremote.port=%d -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false", KafkaPort))
	kafkaCmd := exec.Command(filepath.Join(kafkaDir, "bin/kafka-server-start.sh"), configFile)
	if err := kafkaCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Kafka with JMX: %v", err)
	}
	log.Printf("Started Kafka process with PID: %d", kafkaCmd.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("READY", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		return fmt.Errorf("post-start Kafka status check failed for %s: %v", version, err)
	}
	kafkaCmd.Process.Kill()
	if zookeeperCmd != nil {
		zookeeperCmd.Process.Kill()
	}
	time.Sleep(util.Sleep)
	exec.Command("rm", "-rf", "/tmp/zookeeper").Run()
	exec.Command("rm", "-rf", "/tmp/kafka-logs").Run()
	os.Unsetenv("KAFKA_JMX_OPTS")

	return nil
}

func isKafkaVersionUnder4(version string) bool {
	parts := strings.Split(version, "-")
	if len(parts) < 2 {
		return false
	}

	versionPart := strings.TrimSuffix(parts[1], ".tgz")
	majorVersion := strings.Split(versionPart, ".")[0]
	major, err := strconv.Atoi(majorVersion)
	if err != nil {
		return false
	}

	return major < 4
}
