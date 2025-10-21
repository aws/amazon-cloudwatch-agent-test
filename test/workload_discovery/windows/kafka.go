// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package windows

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/windows/util"
)

const KafkaPort = 9999

func RunKafkaTest() error {
	versions := []string{"kafka_2.13-3.5.0", "kafka_2.13-4.0.0"}

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
	defer func() {
		time.Sleep(util.ShortSleep)
		exec.Command("powershell", "-Command", fmt.Sprintf("Remove-Item -Path 'C:\\tmp\\%s*' -Recurse -Force -ErrorAction SilentlyContinue", version)).Run()
	}()
	// Setup Kafka
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("powershell", "-File", "C:\\scripts.ps1", "Setup-Kafka", "-Version", version, "-Bucket", env.Bucket)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to setup Kafka %s: %v", version, err)
	}
	kafkaDir := fmt.Sprintf("C:\\tmp\\%s", version)
	time.Sleep(util.MediumSleep)

	// Test NEEDS_SETUP phase
	cmd = exec.Command("powershell", "-File", "C:\\scripts.ps1", "Start-Kafka", "-KafkaDir", kafkaDir, "-Version", version)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Kafka without JMX: %v", err)
	}
	time.Sleep(util.MediumSleep)
	if err := util.CheckJavaStatusWithRetry("NEEDS_SETUP/JMX_PORT", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Kafka", "-KafkaDir", kafkaDir, "-Version", version).Run()
		return fmt.Errorf("initial Kafka status check failed for %s: %v", version, err)
	}
	exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Kafka", "-KafkaDir", kafkaDir, "-Version", version).Run()
	time.Sleep(util.MediumSleep)

	// Test READY phase
	cmd = exec.Command("powershell", "-File", "C:\\scripts.ps1", "Start-Kafka", "-KafkaDir", kafkaDir, "-Version", version, "-Port", strconv.Itoa(KafkaPort))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Kafka with JMX: %v", err)
	}
	time.Sleep(util.MediumSleep)
	if err := util.CheckJavaStatusWithRetry("READY", "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Kafka", "-KafkaDir", kafkaDir, "-Version", version).Run()
		return fmt.Errorf("post-start Kafka status check failed for %s: %v", version, err)
	}
	exec.Command("powershell", "-File", "C:\\scripts.ps1", "Stop-Kafka", "-KafkaDir", kafkaDir, "-Version", version).Run()
	time.Sleep(util.LongSleep)

	return nil
}
