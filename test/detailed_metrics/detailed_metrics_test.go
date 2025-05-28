// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"fmt"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
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

	err = setDetailedMetricsFlag()
	require.NoError(t, err, "Error setting detailed metrics flag in agent .yaml")
	log.Printf("Updated detailed metrics flag")

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
	log.Printf("waiting for 5 minutes")
	time.Sleep(5 * time.Minute)
	common.StopAgent()
	end := time.Now()

	// check CWL to ensure we got the expected number of logs in the log stream
	err = awsservice.ValidateLogs(
		"/aws/cwagent", // OTLP -> EMF always go to this log group
		instanceId,
		&start,
		&end,
		awsservice.AssertLogsCount(5),
		awsservice.AssertNoDuplicateLogs(),
	)
	assert.NoError(t, err)
}

func setDetailedMetricsFlag() error {
	ampCommands := []string{
		"sed -ie 's/detailed_metrics: false/detailed_metrics: true/g' /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml",
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
	agentTranslatorCommand := `/opt/aws/amazon-cloudwatch-agent/bin/config-translator \
	--input-dir agent_configs \
	--output /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.toml \
	--mode ec2 -\
	-config /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml \
	--multi-config default`

	out, err := exec.
		Command("bash", "-c", agentTranslatorCommand).
		Output()
	if err != nil {
		log.Printf("Failed to run translator: %s", string(out))
		return err
	}

	return err
}
