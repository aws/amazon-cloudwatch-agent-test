// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"bufio"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var prometheusConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestWriteLogsToCloudWatch writes N number of logs, and then validates that N logs
// are queryable from CloudWatch Logs
func TestDetailedMetricsToEMF(t *testing.T) {
	// this uses the {instance_id} placeholder in the agent configuration,
	// so we need to determine the host's instance ID for validation
	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)

	defer awsservice.DeleteLogGroupAndStream(instanceId, instanceId)

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	setupPrometheus(prometheusConfig, prometheusMetrics)

	agentTranslatorCommand := `/opt/aws/amazon-cloudwatch-agent/bin/config-translator --input resources/agent_config.json \
	--input-dir /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.d \
	--output /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
	--mode ec2 --config /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml \
	--multi-config default`

	out, err := exec.
		Command("bash", "-c", agentTranslatorCommand).
		Output()
	require.NoError(t, err, fmt.Sprint(err)+string(out))
	log.Printf("Translator ran")

	setDetailedMetricsFlag()

	agentStartWithoutTranslatorCommand := `/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent \
		-envconfig /opt/aws/amazon-cloudwatch-agent/etc/env-config.json \
		-config /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
		-otelconfig /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml \
		-pidfile /opt/aws/amazon-cloudwatch-agent/var/amazon-cloudwatch-agent.pid`

	out, err = exec.
		Command("bash", "-c", agentStartWithoutTranslatorCommand).
		Output()

	require.NoError(t, err, fmt.Sprint(err)+string(out))
	log.Printf("Agent has started")

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	time.Sleep(5 * time.Minute)
	common.StopAgent()
	end := time.Now()

	// check CWL to ensure we got the expected number of logs in the log stream
	err = awsservice.ValidateLogs(
		instanceId,
		instanceId,
		&start,
		&end,
		awsservice.AssertLogsCount(5),
		awsservice.AssertNoDuplicateLogs(),
	)
	assert.NoError(t, err)
}

func setDetailedMetricsFlag() {
	// add detailed metrics flag
	filename := "/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml"
	input, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer input.Close()

	output, err := os.Create(filename + ".tmp")
	if err != nil {
		fmt.Printf("Error creating temporary file: %v\n", err)
		return
	}
	defer output.Close()

	scanner := bufio.NewScanner(input)
	writer := bufio.NewWriter(output)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.Replace(line, "detailed_metrics: false", "detailed_metrics: true", 1)
		fmt.Fprintln(writer, line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	writer.Flush()

	if err := os.Rename(filename+".tmp", filename); err != nil {
		fmt.Printf("Error replacing file: %v\n", err)
		return
	}
	//
}

func setupPrometheus(prometheusConfig, prometheusMetrics string) error {
	commands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	err := common.RunCommands(commands)
	if err != nil {
		return fmt.Errorf("failed to setup Prometheus: %v", err)
	}

	// Wait for server to start
	time.Sleep(2 * time.Second)
	return nil
}

func cleanup(logGroupName string) {
	commands := []string{
		"sudo pkill -f 'python3 -m http.server 8101'",
		"sudo rm -f /tmp/prometheus.yaml /tmp/metrics",
	}

	if err := common.RunCommands(commands); err != nil {
		log.Printf("failed to cleanup: %v", err)
	}

	awsservice.DeleteLogGroup(logGroupName)
}
