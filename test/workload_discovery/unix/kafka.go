// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package unix

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/workload_discovery/unix/util"
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
		time.Sleep(util.RaceConditionSleep)
		os.RemoveAll("/tmp/kafka")
	}()
	// Setup Kafka
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("./unix/util/scripts", "setup_kafka", version, env.Bucket)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to setup Kafka %s: %v", version, err)
	}
	kafkaDir := fmt.Sprintf("/tmp/kafka/%s", version)
	time.Sleep(util.RaceConditionSleep)

	// Test NEEDS_SETUP phase
	cmd = exec.Command("./unix/util/scripts", "spin_up_kafka", kafkaDir, version)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Kafka without JMX: %v", err)
	}
	time.Sleep(util.WorkloadUptimeSleep)
	if err := util.CheckStatusWithRetry(util.JMXSetupStatus, "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		exec.Command("./unix/util/scripts", "tear_down_kafka", kafkaDir, version).Run()
		return fmt.Errorf("initial Kafka status check failed for %s: %v", version, err)
	}
	exec.Command("./unix/util/scripts", "tear_down_kafka", kafkaDir, version).Run()
	time.Sleep(util.RaceConditionSleep)

	// Test READY phase
	cmd = exec.Command("./unix/util/scripts", "spin_up_kafka", kafkaDir, version, strconv.Itoa(KafkaPort))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Kafka with JMX: %v", err)
	}
	time.Sleep(util.WorkloadUptimeSleep)
	if err := util.CheckStatusWithRetry(util.Ready, "Kafka Broker", "KAFKA/BROKER", KafkaPort); err != nil {
		exec.Command("./unix/util/scripts", "tear_down_kafka", kafkaDir, version).Run()
		return fmt.Errorf("post-start Kafka status check failed for %s: %v", version, err)
	}
	exec.Command("./unix/util/scripts", "tear_down_kafka", kafkaDir, version).Run()
	time.Sleep(util.RaceConditionSleep)

	return nil
}
