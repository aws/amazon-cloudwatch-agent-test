// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package emf_prometheus_job

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go/aws"
	"log"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var prometheusConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

const (
	prometheusNamespace = "PrometheusEMFJobTest"
	jobNamePrefix       = "prometheus_test_job_"
)

var jobName string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestPrometheusEMFJob(t *testing.T) {
	jobName = jobNamePrefix + strconv.Itoa(rand.Intn(100000))
	log.Printf("Using random job name: %s", jobName)

	log.Println("Starting PrometheusEMFJob Test")

	log.Println("Setting up Prometheus...")
	setupPrometheus(t)

	log.Println("Starting CloudWatch Agent...")
	startAgent(t)

	log.Println("Verifying metrics in correct log group...")
	verifyMetricsInLogGroup(t)

	log.Println("Verifying metrics in CloudWatch...")
	verifyMetricsInCloudWatch(t)

	log.Println("Cleaning up resources...")
	cleanup(t)

	log.Println("PrometheusEMFJob Test completed successfully")
}

func setupPrometheus(t *testing.T) {
	configWithJobName := strings.Replace(prometheusConfig,
		"job_name: 'prometheus_test_job'",
		fmt.Sprintf("job_name: '%s'", jobName),
		1)

	commands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", configWithJobName),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	log.Println("Running Prometheus setup commands...")
	err := common.RunCommands(commands)
	if err != nil {
		log.Printf("Failed to setup Prometheus: %v", err)
		if _, err := common.RunCommand("ls -l /tmp/prometheus_config.yaml"); err != nil {
			log.Printf("prometheus_config.yaml not found: %v", err)
		}
		if _, err := common.RunCommand("ls -l /tmp/metrics"); err != nil {
			log.Printf("metrics file not found: %v", err)
		}
	}
	require.NoError(t, err, "Failed to setup Prometheus")
}

func startAgent(t *testing.T) {
	log.Println("Copying agent configuration...")
	common.CopyFile(filepath.Join("agent_configs", "prometheus_job_config.json"), common.ConfigOutputPath)

	log.Println("Starting CloudWatch Agent...")
	err := common.StartAgent(common.ConfigOutputPath, true, false)
	if err != nil {
		log.Printf("Failed to start agent: %v", err)
		if output, err := common.RunCommand("sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a status"); err != nil {
			log.Printf("Agent status check failed: %v\nOutput: %s", err, output)
		}
	}
	require.NoError(t, err)

	log.Println("Waiting for metrics to be published...")
	time.Sleep(2 * time.Minute)
}

func verifyMetricsInLogGroup(t *testing.T) {
	log.Printf("Checking for existence of log group %s...", jobName)

	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		if awsservice.IsLogGroupExists(jobName) {
			log.Printf("Log group %s exists", jobName)
			return
		}
		log.Printf("Log group %s not found, attempt %d/%d, waiting...", jobName, i+1, maxAttempts)
		time.Sleep(30 * time.Second)
	}

	require.Fail(t, fmt.Sprintf("Log group %s was not created after %d attempts", jobName, maxAttempts))
}

func verifyMetricsInCloudWatch(t *testing.T) {
	metricsToCheck := []struct {
		name     string
		promType string
	}{
		{"prometheus_test_counter", "counter"},
		{"prometheus_test_gauge", "gauge"},
		{"prometheus_test_summary_sum", "summary"},
	}

	valueFetcher := metric.MetricValueFetcher{}

	for _, m := range metricsToCheck {
		log.Printf("Checking metric %s of type %s...", m.name, m.promType)

		dims := []types.Dimension{
			{
				Name:  aws.String("prom_type"),
				Value: aws.String(m.promType),
			},
		}

		values, err := valueFetcher.Fetch(prometheusNamespace, m.name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
		if err != nil {
			log.Printf("Failed to fetch metric %s: %v", m.name, err)
		}
		require.NoError(t, err, fmt.Sprintf("Failed to fetch metric %s", m.name))

		if len(values) == 0 {
			log.Printf("No values found for metric %s", m.name)
		} else {
			log.Printf("Found %d values for metric %s: %v", len(values), m.name, values)
		}
		require.NotEmpty(t, values, fmt.Sprintf("No values found for metric %s", m.name))
	}
}

func cleanup(t *testing.T) {
	log.Println("Running cleanup commands...")
	commands := []string{
		"sudo pkill -f 'python3 -m http.server 8101'",
		"sudo rm -f /tmp/prometheus_config.yaml /tmp/metrics",
	}
	err := common.RunCommands(commands)
	if err != nil {
		log.Printf("Cleanup failed: %v", err)
	}
	require.NoError(t, err, "Failed to cleanup")

	log.Printf("Deleting log group: %s", jobName)
	awsservice.DeleteLogGroup(jobName)
}
