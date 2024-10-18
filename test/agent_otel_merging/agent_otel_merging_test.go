// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package agent_otel_merging

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

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// Tests merging of agent and OTEL yaml
// Merges OTLP receiver to Agent yaml
// And Checks if test_metric was generated(YAML merged successfully)
func TestOtelMerging(t *testing.T) {
	startAgent(t)
	appendOtelConfig(t)
	sendPayload(t)
	verifyMetricsInCloudWatch(t)
	verifyHealthCheck(t)
}

func startAgent(t *testing.T) {
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(10 * time.Second) // Wait for the agent to start properly
}

func appendOtelConfig(t *testing.T) {
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "append-config", "-s", "-m", "ec2", "-c", "file:"+filepath.Join("resources", "otel.yaml"))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to append OTEL config: %s", output)
	time.Sleep(10 * time.Second) // Wait for the agent to apply the new configuration
}

func sendPayload(t *testing.T) {
	currentTimestamp := time.Now().UnixNano()

	payload, err := os.ReadFile(filepath.Join("resources", "metrics.json"))
	require.NoError(t, err, "Failed to read JSON payload from metrics.json")

	payloadStr := string(payload)
	payloadStr = strings.ReplaceAll(payloadStr, "$CURRENT_TIMESTAMP", fmt.Sprintf("%d", currentTimestamp))
	payloadStr = strings.ReplaceAll(payloadStr, "$INSTANCE_ID", environment.GetEnvironmentMetaData().InstanceId)

	req, err := http.NewRequest("POST", "http://localhost:4318/v1/metrics", bytes.NewBuffer([]byte(payloadStr)))
	require.NoError(t, err, "Failed to create HTTP request")

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to send HTTP request")

	defer resp.Body.Close()

	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300, "Unexpected status code: %d", resp.StatusCode)
	time.Sleep(2 * time.Minute)
}

func verifyMetricsInCloudWatch(t *testing.T) {
	metricName := "test_metric"
	namespace := "CWAgent-testing-otel"

	serviceDimName := "service.name"
	serviceDimValue := "test-service"
	instanceDimName := "instance_id"
	instanceDimValue := environment.GetEnvironmentMetaData().InstanceId

	dimensions := []types.Dimension{
		{
			Name:  &serviceDimName,
			Value: &serviceDimValue,
		},
		{
			Name:  &instanceDimName,
			Value: &instanceDimValue,
		},
	}

	startTime := time.Now().Add(-5 * time.Minute)
	endTime := time.Now()
	periodInSeconds := int32(1)
	statType := []types.Statistic{types.StatisticAverage}

	resp, err := awsservice.GetMetricStatistics(
		metricName,
		namespace,
		dimensions,
		startTime,
		endTime,
		periodInSeconds,
		statType,
		nil,
	)

	require.NoError(t, err, "Failed to fetch metric from CloudWatch")
	require.NotEmpty(t, resp.Datapoints, "No data points found for the metric")

}

func verifyHealthCheck(t *testing.T) {
	endpoint := "http://localhost:13133/health/status"

	resp, err := http.Get(endpoint)
	require.NoError(t, err, "Failed to send HTTP request")

	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode, "Expected HTTP status code 200, got %d", resp.StatusCode)
}
