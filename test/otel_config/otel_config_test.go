// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otel_config

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	agentStartupWait  = 5 * time.Second
	metricIngestion   = 30 * time.Second
	namespace         = "OtelConfigTest"
	metricName        = "otel_config_test_metric"
	otlpEndpoint      = "http://localhost:4318/v1/metrics"
	validationRetries = 12
	retryInterval     = 5 * time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestOtelMerging tests merging of agent JSON config and OTEL YAML.
func TestOtelMerging(t *testing.T) {
	defer stopAgent()
	startAgentWithJSON(t)
	appendOtelConfig(t, prepareConfigWithSubstitutions(t, "otel.yaml", ""))
	runMetricValidation(t, "otel_merging")
}

// TestYAMLOnlyConfig tests that the agent can run with only YAML configuration.
func TestYAMLOnlyConfig(t *testing.T) {
	defer stopAgent()
	startAgentWithYAMLOnly(t, prepareConfigWithSubstitutions(t, "yaml_only.yaml", ""))
	runMetricValidation(t, "yaml_only")
}

// TestYAMLOnlyMultipleConfigs tests fetch-config + append-config with multiple YAML files.
// Validates append by writing to a log file and verifying it appears in CloudWatch Logs.
func TestYAMLOnlyMultipleConfigs(t *testing.T) {
	instanceId := environment.GetEnvironmentMetaData().InstanceId
	logGroup := namespace + "/" + instanceId
	logStream := "yaml-only-append"
	testLogFile := filepath.Join(t.TempDir(), "test.log")
	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)
	defer stopAgent()

	startAgentWithYAMLOnly(t, prepareConfigWithSubstitutions(t, "yaml_only.yaml", testLogFile))
	appendOtelConfig(t, prepareConfigWithSubstitutions(t, "yaml_only_append.yaml", testLogFile))

	// Write test log before starting validations
	testStart := time.Now()
	testMessage := fmt.Sprintf("otel_config_append_test_%d", testStart.UnixNano())
	require.NoError(t, os.WriteFile(testLogFile, []byte(testMessage+"\n"), 0644))

	// Run metrics and logs validation in parallel
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sendPayload(t, "yaml_only_multi")
		time.Sleep(metricIngestion)
		verifyMetricsInCloudWatch(t, "yaml_only_multi")
	}()

	go func() {
		defer wg.Done()
		assert.NoError(t, verifyLogsInCloudWatch(logGroup, logStream, testMessage, testStart), "Logs validation failed")
	}()

	wg.Wait()
}

func prepareConfigWithSubstitutions(t *testing.T, configFile, logFile string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("resources", configFile))
	require.NoError(t, err)
	contentStr := strings.ReplaceAll(string(content), "$INSTANCE_ID", environment.GetEnvironmentMetaData().InstanceId)
	contentStr = strings.ReplaceAll(contentStr, "$LOG_FILE", logFile)

	tmpFile := filepath.Join(t.TempDir(), configFile)
	require.NoError(t, os.WriteFile(tmpFile, []byte(contentStr), 0644))
	return tmpFile
}

func verifyLogsInCloudWatch(logGroup, logStream, expectedMessage string, since time.Time) error {
	var err error
	for i := 0; i < validationRetries; i++ {
		time.Sleep(retryInterval)
		err = awsservice.ValidateLogs(logGroup, logStream, &since, nil,
			func(events []cwltypes.OutputLogEvent) error {
				for _, event := range events {
					if strings.Contains(*event.Message, expectedMessage) {
						return nil
					}
				}
				return fmt.Errorf("expected message not found in logs")
			})
		if err == nil {
			return nil
		}
	}
	return err
}

func runMetricValidation(t *testing.T, testName string) {
	t.Helper()
	sendPayload(t, testName)
	time.Sleep(metricIngestion)
	verifyMetricsInCloudWatch(t, testName)
}

func startAgentWithJSON(t *testing.T) {
	t.Helper()
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(agentStartupWait)
}

func startAgentWithYAMLOnly(t *testing.T, configPath string) {
	t.Helper()
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "fetch-config", "-m", "ec2", "-s", "-c", "file:"+configPath)
	output, err := cmd.CombinedOutput()
	t.Logf("fetch-config output: %s", output)
	require.NoError(t, err, "Failed to start agent with YAML config: %s", output)
	require.Contains(t, string(output), "Configuration validation first phase skipped",
		"Expected YAML-only mode skip message")
	time.Sleep(agentStartupWait)
}

func appendOtelConfig(t *testing.T, configPath string) {
	t.Helper()
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "append-config", "-s", "-m", "ec2", "-c", "file:"+configPath)
	output, err := cmd.CombinedOutput()
	t.Logf("append-config output: %s", output)
	require.NoError(t, err, "Failed to append OTEL config: %s", output)
	time.Sleep(agentStartupWait)
}

func stopAgent() {
	exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "stop").Run()
}

func sendPayload(t *testing.T, testName string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("resources", "metrics.json"))
	require.NoError(t, err, "Failed to read payload file")

	payloadStr := string(payload)
	payloadStr = strings.ReplaceAll(payloadStr, "$CURRENT_TIMESTAMP", fmt.Sprintf("%d", time.Now().UnixNano()))
	payloadStr = strings.ReplaceAll(payloadStr, "$INSTANCE_ID", environment.GetEnvironmentMetaData().InstanceId)
	payloadStr = strings.ReplaceAll(payloadStr, "$TEST_NAME", testName)

	req, err := http.NewRequest("POST", otlpEndpoint, bytes.NewBuffer([]byte(payloadStr)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: retryInterval}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to send payload")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Unexpected status code")
}

func verifyMetricsInCloudWatch(t *testing.T, testName string) {
	t.Helper()
	instanceDimValue := environment.GetEnvironmentMetaData().InstanceId

	dimensionsFilter := []cwtypes.DimensionFilter{
		{Name: aws.String("test_name"), Value: aws.String(testName)},
		{Name: aws.String("instance_id"), Value: aws.String(instanceDimValue)},
	}

	awsservice.ValidateMetricWithTest(t, metricName, namespace, dimensionsFilter, validationRetries, retryInterval)
}
