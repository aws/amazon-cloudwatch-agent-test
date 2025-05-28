// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

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

	// Since there is no way to set the detailed_metrics flag on the awsemfexporter using the agent's json configuration,
	// run the translator, modify the yaml, and then run the agent
	err := runTranslator()
	require.NoError(t, err, "Error running translator")
	log.Printf("Translator ran")

	err = modifyAgentYaml()
	require.NoError(t, err, "Error modifying settings in agent .yaml")
	log.Printf("Updated agent yaml file")

	agentStartWithoutTranslatorCommand := `/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent \
		-envconfig /opt/aws/amazon-cloudwatch-agent/etc/env-config.json \
		-config /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
		-otelconfig /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml \
		-pidfile /opt/aws/amazon-cloudwatch-agent/var/amazon-cloudwatch-agent.pid &`

	common.RunAsyncCommand(agentStartWithoutTranslatorCommand)
	log.Printf("Agent has started")

	common.RunAsyncCommand("resources/otlp_pusher.sh")

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	waittime := 1 * time.Minute
	log.Printf("waiting for %s", waittime)
	time.Sleep(waittime)
	common.StopAgent()
	end := time.Now()

	// check CWL to ensure we got the expected number of logs in the log stream
	err = awsservice.ValidateLogs(
		"/aws/cwagent", // OTLP -> EMF always go to this log group
		instanceId,
		&start,
		&end,
		validateLogs,
	)
	assert.NoError(t, err)
}

func modifyAgentYaml() error {
	instanceId := awsservice.GetInstanceId()
	ampCommands := []string{
		"sed -ie 's/detailed_metrics: false/detailed_metrics: true/g' /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml",
		// translator does not move logs::log_stream_name setting to the emf exporter so do it ourselves to ensure the log stream is created
		fmt.Sprintf(`sed -ie 's/log_stream_name: ""/log_stream_name: "%s"/g' /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml`, instanceId),
	}
	err := common.RunCommands(ampCommands)
	if err != nil {
		return fmt.Errorf("failed to modify agent configuration: %w", err)
	}
	return nil
}

func runTranslator() error {
	// translator only works on .tmp files, so copy what we have to a .tmp
	common.CopyFile("agent_configs/agent_config.json", "agent_configs/agent_config.json.tmp")
	agentTranslatorCommands := []string{
		`/opt/aws/amazon-cloudwatch-agent/bin/config-translator \
	--input-dir /home/ec2-user/amazon-cloudwatch-agent-test/test/detailed_metrics/agent_configs \
	--output /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
	--mode ec2 \
	--config /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml \
	--multi-config default`,
	}

	err := common.RunCommands(agentTranslatorCommands)
	if err != nil {
		log.Printf("Failed to run translator: %s", err)
		return err
	}

	return err
}

func validateLogs(events []types.OutputLogEvent) error {
	// we expect:
	// * 4 log events around the same time
	// * 1 log event for each quantile (.5, .9, .95) for 3 total
	// * 1 log event for sum/count
	foundQuantiles := make(map[string]bool)
	foundSumCount := false

	// just pull the last 4 events
	for _, event := range events[0:4] {
		var logData map[string]interface{}
		if err := json.Unmarshal([]byte(*event.Message), &logData); err != nil {
			return fmt.Errorf("failed to parse log event: %v", err)
		}

		// Check for quantile events
		if quantile, exists := logData["quantile"].(string); exists {
			// Validate the quantile value
			switch quantile {
			case "0.5", "0.95", "0.99":
				foundQuantiles[quantile] = true

				// Verify my.summary exists and is a number
				if _, ok := logData["my.summary"].(float64); !ok {
					return fmt.Errorf("my.summary not found or not a number for quantile %s", quantile)
				}
			default:
				return fmt.Errorf("unexpected quantile value: %s", quantile)
			}
		} else {
			// This should be the sum/count event
			sumValue, hasSum := logData["my.summary_sum"].(float64)
			countValue, hasCount := logData["my.summary_count"].(float64)

			if hasSum && hasCount {
				foundSumCount = true

				// Optional: Add specific value validations if needed
				if countValue <= 0 {
					return fmt.Errorf("invalid count value: %f", countValue)
				}
				if sumValue < 0 {
					return fmt.Errorf("invalid sum value: %f", sumValue)
				}
			}
		}
	}

	// Verify we found all expected events
	if !foundSumCount {
		return fmt.Errorf("missing sum/count metrics")
	}

	expectedQuantiles := []string{"0.5", "0.95", "0.99"}

	for _, expectedQuantile := range expectedQuantiles {
		if !foundQuantiles[expectedQuantile] {
			return fmt.Errorf("missing quantile %s", expectedQuantile)
		}
	}

	return nil
	return nil
}
