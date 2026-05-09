// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Tests for the Application Signals OTLP logs pipeline: dynamic per-service
// log group/stream routing via the CW OTLP endpoint.
//
// Uses CWAgent JSON config → translator → OTel YAML (the standard customer path).
//
// Test cases:
//   - TestAppSignalsLogsDynamicRouting: Multiple services route to separate log groups
//   - TestAppSignalsLogsNoisyNeighbor: Service B's creation fails, Service A is not blocked
//   - TestAppSignalsLogsDefaultPlaceholder: Logs without service.name use fallback
package app_signals

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	sleepForFlush    = 30 * time.Second
	logGroupPrefix   = "/test/otlp-dynamic/"

	// Resource attribute values used in test logs.
	// log_stream_name template is "{host.name}/{service.instance.id}"
	testHostName   = "test-host"
	testInstanceID = "instance-001"
	testStreamName = testHostName + "/" + testInstanceID
)

// TestAppSignalsLogsDynamicRouting verifies that logs from different services are
// routed to separate, dynamically-created log groups.
func TestAppSignalsLogsDynamicRouting(t *testing.T) {
	services := []string{"service-a", "service-b", "service-c"}
	numLogsPerService := 5

	for _, svc := range services {
		defer awsservice.DeleteLogGroupAndStream(logGroupPrefix+svc, testStreamName)
	}

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	common.CopyFile("agent_configs/config_logs_dynamic_placeholders.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	require.NoError(t, err, "Failed to start agent")
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	for _, svc := range services {
		sendOTLPLogs(t, svc, numLogsPerService)
	}

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	agentLog, _ := os.ReadFile(common.AgentLogFile)
	t.Logf("Agent logs (tail):\n%s", tail(string(agentLog), 2000))

	for _, svc := range services {
		logGroup := logGroupPrefix + svc
		t.Run(svc, func(t *testing.T) {
			// Verify the log stream name was resolved correctly from placeholders
			// Template: "{host.name}/{service.instance.id}" → "test-host/instance-001"
			streams := awsservice.GetLogStreams(logGroup)
			require.NotEmpty(t, streams, "No log streams found in %s", logGroup)
			assert.Equal(t, testStreamName, *streams[0].LogStreamName,
				"Log stream name should be resolved from {host.name}/{service.instance.id} placeholders")

			err := awsservice.ValidateLogs(
				logGroup,
				testStreamName,
				&start,
				&end,
				awsservice.AssertLogsCount(numLogsPerService),
				awsservice.AssertLogsNotEmpty(),
			)
			assert.NoError(t, err, "Failed to validate logs for service %s in log group %s", svc, logGroup)
		})
	}
}

// TestAppSignalsLogsNoisyNeighbor verifies that when one service's log group cannot be created
// (e.g., due to an invalid name), logs from other services are not blocked.
func TestAppSignalsLogsNoisyNeighbor(t *testing.T) {
	goodService := "healthy-service"
	badService := "bad:service:name"

	goodLogGroup := logGroupPrefix + goodService
	badLogGroup := logGroupPrefix + badService

	defer awsservice.DeleteLogGroupAndStream(goodLogGroup, testStreamName)
	defer awsservice.DeleteLogGroupAndStream(badLogGroup, testStreamName)

	numLogs := 10

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	common.CopyFile("agent_configs/config_logs_dynamic_placeholders.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	require.NoError(t, err, "Failed to start agent")
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	for i := 0; i < numLogs; i++ {
		sendOTLPLogs(t, goodService, 1)
		sendOTLPLogs(t, badService, 1)
	}

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	agentLog, _ := os.ReadFile(common.AgentLogFile)
	t.Logf("Agent logs (tail):\n%s", tail(string(agentLog), 2000))

	t.Run("healthy_service_not_blocked", func(t *testing.T) {
		err := awsservice.ValidateLogs(
			goodLogGroup,
			"default",
			&start,
			&end,
			awsservice.AssertLogsCount(numLogs),
		)
		assert.NoError(t, err,
			"Healthy service logs were blocked by noisy neighbor! "+
				"Expected %d logs in %s but validation failed", numLogs, goodLogGroup)
	})

	t.Run("bad_service_loggroup_not_created", func(t *testing.T) {
		exists := awsservice.IsLogGroupExists(badLogGroup)
		assert.False(t, exists,
			"Bad service log group %s should not exist", badLogGroup)
	})

	t.Run("agent_logs_show_creation_failure", func(t *testing.T) {
		agentLogStr := string(agentLog)
		if !strings.Contains(agentLogStr, "caller") {
			t.Skip("Agent log file does not contain OTel-level logs")
		}
		assert.Contains(t, agentLogStr, "Failed to create log group",
			"Agent logs should contain creation failure message for bad service")
	})
}

// TestAppSignalsLogsDefaultPlaceholder verifies that logs sent without a service.name
// resource attribute are routed to the fallback log group. The transform
// processor's conditional OTTL statement sets the log group to
// "/test/otlp-dynamic/undefined" when service.name is missing.
func TestAppSignalsLogsDefaultPlaceholder(t *testing.T) {
	defaultLogGroup := logGroupPrefix + "undefined"
	numLogs := 5

	defer awsservice.DeleteLogGroupAndStream(defaultLogGroup, testStreamName)

	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	common.CopyFile("agent_configs/config_logs_dynamic_placeholders.json", configOutputPath)

	err := common.StartAgent(configOutputPath, true, false)
	require.NoError(t, err, "Failed to start agent")
	defer common.StopAgent()

	time.Sleep(sleepForFlush)
	start := time.Now()

	sendOTLPLogsNoService(t, numLogs)

	time.Sleep(sleepForFlush)
	common.StopAgent()
	end := time.Now()

	err = awsservice.ValidateLogs(
		defaultLogGroup,
		"default",
		&start,
		&end,
		awsservice.AssertLogsCount(numLogs),
		awsservice.AssertLogsNotEmpty(),
	)
	assert.NoError(t, err,
		"Logs without service.name should route to default log group %s", defaultLogGroup)
}

func sendOTLPLogs(t *testing.T, serviceName string, numLogs int) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_logs.sh",
		serviceName, fmt.Sprintf("%d", numLogs), testHostName, testInstanceID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_logs.sh for %s returned error: %v\nOutput: %s", serviceName, err, string(output))
	}
}

func sendOTLPLogsNoService(t *testing.T, numLogs int) {
	t.Helper()
	cmd := exec.Command("/bin/bash", "resources/send_otlp_logs_no_service.sh", fmt.Sprintf("%d", numLogs))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: send_otlp_logs_no_service.sh returned error: %v\nOutput: %s", err, string(output))
	}
}

// tail returns the last n bytes of s.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...\n" + s[len(s)-n:]
}
