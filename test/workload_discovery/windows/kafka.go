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

	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
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
	kafkaDir := fmt.Sprintf("C:\\temp\\%s", version[:len(version)-4])
	var zookeeperCmd *exec.Cmd
	var configFile string

	if isKafkaVersionUnder4(version) {
		zookeeperCmd = exec.Command(kafkaDir+"\\bin\\windows\\zookeeper-server-start.bat",
			kafkaDir+"\\config\\zookeeper.properties")
		zookeeperCmd.Env = append(os.Environ(), "KAFKA_HEAP_OPTS=-Xmx128M -Xms64M")
		zookeeperCmd.Start()
		time.Sleep(util.Sleep)

		minimalConfig := `broker.id=0
listeners=PLAINTEXT://localhost:9092
log.dirs=C:/temp/kafka-logs
zookeeper.connect=localhost:2181
log.cleaner.enable=false
num.network.threads=1
num.io.threads=2
socket.send.buffer.bytes=32768
socket.receive.buffer.bytes=32768
socket.request.max.bytes=1048576
log.segment.bytes=16777216
log.retention.hours=1
log.retention.check.interval.ms=60000
offsets.topic.num.partitions=1
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
`
		configFile = kafkaDir + "\\config\\minimal.properties"
		if err := os.WriteFile(configFile, []byte(minimalConfig), 0644); err != nil {
			return fmt.Errorf("failed to create minimal config: %v", err)
		}
	} else {
		kraftConfig := `process.roles=broker,controller
node.id=0
controller.quorum.voters=0@localhost:9093
listeners=PLAINTEXT://localhost:9092,CONTROLLER://localhost:9093
inter.broker.listener.name=PLAINTEXT
advertised.listeners=PLAINTEXT://localhost:9092
controller.listener.names=CONTROLLER
log.dirs=C:/temp/kafka-logs
num.network.threads=3
num.io.threads=8
socket.send.buffer.bytes=102400
socket.receive.buffer.bytes=102400
socket.request.max.bytes=104857600
num.partitions=1
num.recovery.threads.per.data.dir=1
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
log.retention.hours=1
log.segment.bytes=1073741824
log.retention.check.interval.ms=300000
`
		configFile = kafkaDir + "\\config\\kraft-server.properties"
		if err := os.WriteFile(configFile, []byte(kraftConfig), 0644); err != nil {
			return fmt.Errorf("failed to create KRaft config: %v", err)
		}
		uuidCmd := exec.Command(kafkaDir+"\\bin\\windows\\kafka-storage.bat", "random-uuid")
		uuidCmd.Dir = kafkaDir
		uuidOutput, err := uuidCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to generate cluster UUID: %v, output: %s", err, string(uuidOutput))
		}
		clusterUUID := strings.TrimSpace(string(uuidOutput))
		log.Printf("Generated cluster UUID: %s", clusterUUID)
		exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kraft-controller-logs").Run()
		exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kafka-logs").Run()
		os.MkdirAll("C:\\temp\\kafka-logs", 0755)
		formatCmd := exec.Command(kafkaDir+"\\bin\\windows\\kafka-storage.bat",
			"format", "-t", clusterUUID, "-c", configFile, "--ignore-formatted")
		formatCmd.Dir = kafkaDir
		if output, err := formatCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to format Kafka storage: %v, output: %s", err, string(output))
		}
		log.Printf("Formatted KRaft storage successfully")
	}

	// Test NEEDS_SETUP phase
	kafkaCmdNoJMX := exec.Command(kafkaDir+"\\bin\\windows\\kafka-server-start.bat", configFile)
	heapOpts := "KAFKA_HEAP_OPTS=-Xmx128M -Xms64M"
	if strings.Contains(version, "4.0.0") {
		heapOpts = "KAFKA_HEAP_OPTS=-Xmx512M -Xms256M"
	}
	kafkaCmdNoJMX.Env = append(os.Environ(), heapOpts)
	kafkaCmdNoJMX.Start()
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("NEEDS_SETUP/JMX_PORT", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		return fmt.Errorf("initial Kafka status check failed for %s: %v", version, err)
	}
	exec.Command(kafkaDir + "\\bin\\windows\\kafka-server-stop.bat").Run()
	if isKafkaVersionUnder4(version) {
		exec.Command(kafkaDir + "\\bin\\windows\\zookeeper-server-stop.bat").Run()
	}
	exec.Command("taskkill", "/F", "/IM", "java.exe").Run()
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\zookeeper").Run()
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kafka-logs").Run()
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kraft-controller-logs").Run()
	if isKafkaVersionUnder4(version) {
		zookeeperCmd = exec.Command(kafkaDir+"\\bin\\windows\\zookeeper-server-start.bat",
			kafkaDir+"\\config\\zookeeper.properties")
		zookeeperCmd.Env = append(os.Environ(), "KAFKA_HEAP_OPTS=-Xmx128M -Xms64M")
		zookeeperCmd.Start()
		time.Sleep(util.Sleep)
	}
	os.Setenv("KAFKA_JMX_OPTS", fmt.Sprintf("-Dcom.sun.management.jmxremote.port=%d -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false", KafkaPort))

	// Test READY phase
	kafkaCmd := exec.Command(kafkaDir+"\\bin\\windows\\kafka-server-start.bat", configFile)
	heapOpts = "KAFKA_HEAP_OPTS=-Xmx128M -Xms64M"
	if strings.Contains(version, "4.0.0") {
		heapOpts = "KAFKA_HEAP_OPTS=-Xmx512M -Xms256M"
	}
	kafkaCmd.Env = append(os.Environ(), heapOpts)
	if err := kafkaCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Kafka with JMX: %v", err)
	}
	log.Printf("Started Kafka process with PID: %d", kafkaCmd.Process.Pid)
	time.Sleep(util.Sleep)
	if err := util.CheckJavaStatus("READY", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		return fmt.Errorf("post-start Kafka status check failed for %s: %v", version, err)
	}
	exec.Command(kafkaDir + "\\bin\\windows\\kafka-server-stop.bat").Run()
	if zookeeperCmd != nil {
		exec.Command(kafkaDir + "\\bin\\windows\\zookeeper-server-stop.bat").Run()
	}
	kafkaCmd.Process.Kill()
	if zookeeperCmd != nil {
		zookeeperCmd.Process.Kill()
	}
	time.Sleep(util.Sleep)
	exec.Command("taskkill", "/F", "/IM", "java.exe").Run()
	time.Sleep(util.Sleep)
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\zookeeper").Run()
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kafka-logs").Run()
	exec.Command("rmdir", "/S", "/Q", "C:\\temp\\kraft-controller-logs").Run()
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
