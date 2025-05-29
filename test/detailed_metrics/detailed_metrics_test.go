// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
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

	err = startAgent()
	require.NoError(t, err, "Unable to start the agent")
	log.Printf("Agent has started")

	err = startOtlpPusher()
	require.NoError(t, err, "Unable to start the OTLP pusher")
	log.Printf("OTLP Pusher started")

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	waittime := 45 * time.Second
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
		"sudo sed -ie 's/detailed_metrics: false/detailed_metrics: true/g' /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml",
		// translator does not move logs::log_stream_name setting to the emf exporter so do it ourselves to ensure the log stream is created
		fmt.Sprintf(`sudo sed -ie 's/log_stream_name: ""/log_stream_name: "%s"/g' /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml`, instanceId),
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
		`sudo /opt/aws/amazon-cloudwatch-agent/bin/config-translator \
	--input-dir /home/ec2-user/amazon-cloudwatch-agent-test/test/detailed_metrics/agent_configs \
	--output /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
	--mode ec2 \
	--config /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml \
	--multi-config default`,
	}

	err := common.RunCommands(agentTranslatorCommands)
	if err != nil {
		return err
	}

	return err
}

func startAgent() error {
	agentStartWithoutTranslatorCommand := `sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent \
		-envconfig /opt/aws/amazon-cloudwatch-agent/etc/env-config.json \
		-config /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
		-otelconfig /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml \
		-pidfile /opt/aws/amazon-cloudwatch-agent/var/amazon-cloudwatch-agent.pid &`

	err := common.RunAsyncCommand(agentStartWithoutTranslatorCommand)
	if err != nil {
		return err
	}
	return nil
}

func startOtlpPusher() error {
	return common.RunAsyncCommand("sudo resources/otlp_pusher.sh")
}

func validateLogs(events []types.OutputLogEvent) error {

	errs := []error{}

	// Each time the OTLP pusher sends metrics to the agent, we expect to see 4 log events:
	// * 1 log event for each quantile (.5, .9, .95) for 3 total
	// * 1 log event for sum/count
	// the agent runs for 45s and the pusher pushes every 30s. so the agent could see 1 or 2 pushes from the OTLP pusher,
	// so we would expect 4 or 8 log events in CloudWatch
	if len(events) != 4 && len(events) != 8 {
		return fmt.Errorf("unexpected number of events. expected 4 or 8, got %d", len(events))
	}

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
					errs = append(errs, fmt.Errorf("my.summary not found or not a number for quantile %s", quantile))
				}
			default:
				errs = append(errs, fmt.Errorf("unexpected quantile value: %s", quantile))
			}
		} else {
			// This should be the sum/count event
			sumValue, hasSum := logData["my.summary_sum"].(float64)
			countValue, hasCount := logData["my.summary_count"].(float64)

			expectedSum := 5000.0
			expectedCount := 100.0

			if hasSum && hasCount {
				foundSumCount = true

				if math.Abs(countValue-expectedCount) > 0.001 {
					errs = append(errs, fmt.Errorf("expected count value %f, got %f", expectedCount, countValue))
				}
				if math.Abs(sumValue-expectedSum) > 0.001 {
					errs = append(errs, fmt.Errorf("expected sum value %f, got %f", expectedSum, sumValue))
				}
			}
		}
	}

	if !foundSumCount {
		errs = append(errs, fmt.Errorf("sum/count event not found"))
	}

	expectedQuantiles := []string{"0.5", "0.95", "0.99"}
	for _, expectedQuantile := range expectedQuantiles {
		if !foundQuantiles[expectedQuantile] {
			errs = append(errs, fmt.Errorf("missing quantile %s", expectedQuantile))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
