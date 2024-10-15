package agent_otel_merging

import (
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// This test runs the agent and creates a metric
func TestOtelMerging(t *testing.T) {

	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))

	//merging otel yaml
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "append-config", "-s", "-m", "ec2", "-c", "file:"+filepath.Join("resources", "otel.yaml"))

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to append config: %s", output)
	time.Sleep(5 * time.Second)

	//Creating Pay
	currentTimestamp := time.Now().UnixNano()
	payload, err := os.ReadFile(filepath.Join("resources", "metrics.json"))
	require.NoError(t, err, "Failed to read JSON payload from config.json")
	payloadStr := string(payload)
	payloadStr = strings.ReplaceAll(payloadStr, "$CURRENT_TIMESTAMP", fmt.Sprintf("%d", currentTimestamp))

	curlCmd := exec.Command("curl", "-X", "POST", "http://localhost:4318/v1/metrics",
		"-H", "Content-Type: application/json", "-d", payloadStr)

	curlOutput, err := curlCmd.CombinedOutput()
	require.NoError(t, err, "Failed to send metrics: %s", curlOutput)

	time.Sleep(1 * time.Minute)

	verifyCmd := exec.Command("aws", "cloudwatch", "get-metric-statistics",
		"--namespace", "CWAgent-testing-otel",
		"--metric-name", "test_metric",
		"--dimensions", "Name=service.name,Value=test-service",
		"--start-time", time.Now().Add(-5*time.Minute).Format("2024-01-02T15:04:05Z"),
		"--end-time", time.Now().Format("2024-01-02T15:04:05Z"),
		"--period", "60", // Period in seconds
		"--statistics", "Average")

	verifyOutput, err := verifyCmd.CombinedOutput()
	require.NoError(t, err, "Failed to fetch metric: %s", verifyOutput)

	type MetricStatistics struct {
		Datapoints []struct {
			Timestamp time.Time `json:"Timestamp"`
			Average   float64   `json:"Average"`
		} `json:"Datapoints"`
		Label string `json:"Label"`
	}

	var stats MetricStatistics
	err = json.Unmarshal(verifyOutput, &stats)
	require.NoError(t, err, "Failed to parse CloudWatch response")

	//Make sure metrics appear
	require.NotEmpty(t, stats.Datapoints, "No data points found for the metric")

	t.Logf("CloudWatch Metric Output: %s", verifyOutput)

	common.StopAgent()
}
