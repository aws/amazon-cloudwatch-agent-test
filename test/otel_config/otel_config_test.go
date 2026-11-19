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

	placeholderInstanceID = "$INSTANCE_ID"
	placeholderLogFile    = "$LOG_FILE"
	placeholderTimestamp  = "$CURRENT_TIMESTAMP"
	placeholderTestName   = "$TEST_NAME"

	testNameFetchJSONAppendYAML = "fetch_json_append_yaml"
	testNameFetchYAML           = "fetch_yaml"
	testNameFetchYAMLAppendJSON = "fetch_yaml_append_json"
	testNameFetchYAMLAppendYAML = "fetch_yaml_append_yaml"

	dirResources    = "resources"
	dirAgentConfigs = "agent_configs"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestFetchJSONAppendYAML tests merging of agent JSON config and OTEL YAML.
func TestFetchJSONAppendYAML(t *testing.T) {
	startAgentWithJSON(t)
	defer common.StopAgent()
	appendConfig(t, prepareConfigWithSubstitutions(t, "otel.yaml", map[string]string{
		placeholderInstanceID: environment.GetEnvironmentMetaData().InstanceId,
	}))
	runMetricValidation(t, testNameFetchJSONAppendYAML)
}

// TestFetchYAML tests that the agent can run with only YAML configuration.
func TestFetchYAML(t *testing.T) {
	startAgentWithYAML(t, prepareConfigWithSubstitutions(t, "yaml_only.yaml", map[string]string{
		placeholderInstanceID: environment.GetEnvironmentMetaData().InstanceId,
	}))
	defer common.StopAgent()
	runMetricValidation(t, testNameFetchYAML)
}

// TestFetchYAMLAppendJSON tests fetch-config with YAML followed by append-config with JSON.
func TestFetchYAMLAppendJSON(t *testing.T) {
	startAgentWithYAML(t, prepareConfigWithSubstitutions(t, "yaml_only.yaml", map[string]string{
		placeholderInstanceID: environment.GetEnvironmentMetaData().InstanceId,
	}))
	defer common.StopAgent()
	appendConfig(t, filepath.Join(dirAgentConfigs, "config.json"))
	runMetricValidation(t, testNameFetchYAMLAppendJSON)
}

// TestFetchYAMLAppendYAML tests fetch-config + append-config with multiple YAML files.
// Validates append by writing to a log file and verifying it appears in CloudWatch Logs.
func TestFetchYAMLAppendYAML(t *testing.T) {
	instanceId := environment.GetEnvironmentMetaData().InstanceId
	logGroup := namespace + "/" + instanceId
	logStream := testNameFetchYAMLAppendYAML
	testLogFile := filepath.Join(t.TempDir(), "test.log")
	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	subs := map[string]string{
		placeholderInstanceID: instanceId,
		placeholderLogFile:    testLogFile,
	}
	startAgentWithYAML(t, prepareConfigWithSubstitutions(t, "yaml_only.yaml", subs))
	defer common.StopAgent()
	appendConfig(t, prepareConfigWithSubstitutions(t, "yaml_only_append.yaml", subs))

	// Write test log before starting validations
	testStart := time.Now()
	testMessage := fmt.Sprintf("%s_%d", testNameFetchYAMLAppendYAML, testStart.UnixNano())
	require.NoError(t, os.WriteFile(testLogFile, []byte(testMessage+"\n"), 0644))

	sendPayload(t, testNameFetchYAMLAppendYAML)
	time.Sleep(metricIngestion)
	verifyMetricsInCloudWatch(t, testNameFetchYAMLAppendYAML)
	assert.NoError(t, verifyLogsInCloudWatch(logGroup, logStream, testMessage, testStart), "Logs validation failed")
}

func prepareConfigWithSubstitutions(t *testing.T, configFile string, substitutions map[string]string) string {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), configFile)
	common.CopyFile(filepath.Join(dirResources, configFile), tmpFile)
	require.NoError(t, common.ReplacePlaceholders(tmpFile, substitutions))
	return tmpFile
}

func verifyLogsInCloudWatch(logGroup, logStream, expectedMessage string, since time.Time) error {
	var err error
	for i := 0; i < validationRetries; i++ {
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
		time.Sleep(retryInterval)
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
	common.CopyFile(filepath.Join(dirAgentConfigs, "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, false, false))
	time.Sleep(agentStartupWait)
}

func startAgentWithYAML(t *testing.T, configPath string) {
	t.Helper()
	require.NoError(t, common.StartAgent(configPath, false, false))
	time.Sleep(agentStartupWait)
}

func appendConfig(t *testing.T, configPath string) {
	t.Helper()
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "append-config", "-s", "-m", "ec2", "-c", "file:"+configPath)
	output, err := cmd.CombinedOutput()
	t.Logf("append-config output: %s", output)
	require.NoError(t, err, "Failed to append config: %s", output)
	time.Sleep(agentStartupWait)
}

func sendPayload(t *testing.T, testName string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(dirResources, "metrics.json"))
	require.NoError(t, err, "Failed to read payload file")

	payloadStr := string(payload)
	payloadStr = strings.ReplaceAll(payloadStr, placeholderTimestamp, fmt.Sprintf("%d", time.Now().UnixNano()))
	payloadStr = strings.ReplaceAll(payloadStr, placeholderInstanceID, environment.GetEnvironmentMetaData().InstanceId)
	payloadStr = strings.ReplaceAll(payloadStr, placeholderTestName, testName)

	req, err := http.NewRequest(http.MethodPost, otlpEndpoint, bytes.NewBuffer([]byte(payloadStr)))
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
