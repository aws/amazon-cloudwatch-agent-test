package amp

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

//go:embed resources/otlp_pusher.sh
var otlpPusherScript string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestOTLPHistogramMetrics(t *testing.T) {
	startAgent(t)
	log.Println("Testing TestOTLPHistogramMetrics")
	instanceID := awsservice.GetInstanceId()
	log.Println("Instance ID", instanceID)

	err := runOTLPPusher(instanceID)
	assert.NoError(t, err)
	dims := []types.Dimension{
		{
			Name:  aws.String("environment"),
			Value: aws.String("production"),
		},
		{
			Name:  aws.String("instance_id"),
			Value: aws.String(instanceID),
		},
		{
			Name:  aws.String("service.name"),
			Value: aws.String("my.service"),
		},
		{
			Name:  aws.String("my.delta.histogram.attr"),
			Value: aws.String("some value"),
		},
		{
			Name:  aws.String("region"),
			Value: aws.String("us-west-2"),
		},
		{
			Name:  aws.String("custom.attribute"),
			Value: aws.String("test-value"),
		},
		{
			Name:  aws.String("status"),
			Value: aws.String("active"),
		},
	}

	fetcher := metric.MetricValueFetcher{}

	namespace := "CWAgent"
	metricName := "my.delta.histogram"

	time.Sleep(2 * time.Minute)

	values, err := fetcher.Fetch(namespace, metricName, dims, "Maximum", metric.HighResolutionStatPeriod)
	assert.NoError(t, err, "Failed to fetch metrics")
	assert.NotEmpty(t, values, "No metric values returned")

	t.Logf("Metrics retrieved from CloudWatch for %s: %v", metricName, values)

	for _, value := range values {
		assert.Equal(t, float64(2), value, fmt.Sprintf("Expected value 2, got %v for metric %s", value, metricName))
	}
}

func startAgent(t *testing.T) {
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(10 * time.Second) // Wait for the agent to start properly
}

func runOTLPPusher(instanceID string) error {
	tmpfile, err := os.CreateTemp("", "otlp_pusher_*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(otlpPusherScript)); err != nil {
		return fmt.Errorf("failed to write script: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %v", err)
	}

	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %v", err)
	}

	cmd := exec.Command(tmpfile.Name())
	cmd.Env = append(os.Environ(), fmt.Sprintf("INSTANCE_ID=%s", instanceID))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script failed: %v, output: %s", err, string(output))
	}

	return nil
}
