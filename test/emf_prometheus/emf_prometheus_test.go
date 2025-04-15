// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package emf_prometheus

import (
	_ "embed"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var prometheusConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

const prometheusNamespace = "PrometheusEMFTest"

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestPrometheusEMF(t *testing.T) {
	log.Println("Starting PrometheusEMF Test")

	log.Println("Setting up Prometheus...")
	setupPrometheus(t)

	log.Println("Starting CloudWatch Agent...")
	startAgent(t)

	log.Println("Verifying untyped metric absence...")
	verifyUntypedMetricAbsence(t)

	log.Println("Verifying other metrics presence...")
	verifyOtherMetricsPresence(t)

	log.Println("Cleaning up resources...")
	cleanup(t)

	log.Println("PrometheusEMF Test completed successfully")
}

func setupPrometheus(t *testing.T) {
	commands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	log.Println("Running Prometheus setup commands...")
	err := common.RunCommands(commands)
	if err != nil {
		log.Printf("Failed to setup Prometheus: %v", err)
		// Verify files were created
		if _, err := common.RunCommand("ls -l /tmp/prometheus_config.yaml"); err != nil {
			log.Printf("prometheus_config.yaml not found: %v", err)
		}
		if _, err := common.RunCommand("ls -l /tmp/metrics"); err != nil {
			log.Printf("metrics file not found: %v", err)
		}
		// Check if Python server is running
		if _, err := common.RunCommand("ps aux | grep 'python3 -m http.server 8101'"); err != nil {
			log.Printf("Python HTTP server not running: %v", err)
		}
	}
	require.NoError(t, err, "Failed to setup Prometheus")

	// Verify HTTP server is responding
	log.Println("Verifying HTTP server is accessible...")
	if _, err := common.RunCommand("curl -s -f http://localhost:8101/metrics"); err != nil {
		log.Printf("WARNING: HTTP server not responding: %v", err)
	}
}

func startAgent(t *testing.T) {
	log.Println("Copying agent configuration...")
	common.CopyFile(filepath.Join("agent_configs", "emf_prometheus_config.json"), common.ConfigOutputPath)

	log.Println("Starting CloudWatch Agent...")
	err := common.StartAgent(common.ConfigOutputPath, true, false)
	if err != nil {
		log.Printf("Failed to start agent: %v", err)
		// Check agent status
		if output, err := common.RunCommand("sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a status"); err != nil {
			log.Printf("Agent status check failed: %v\nOutput: %s", err, output)
		}
	}
	require.NoError(t, err)

	log.Println("Waiting for metrics to be published...")
	time.Sleep(2 * time.Minute)
}

func verifyUntypedMetricAbsence(t *testing.T) {
	dims := []types.Dimension{
		{
			Name:  aws.String("prom_type"),
			Value: aws.String("untyped"),
		},
	}

	log.Printf("Checking for absence of untyped metric in namespace %s...", prometheusNamespace)
	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(prometheusNamespace, "prometheus_test_untyped", dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)

	if err != nil {
		log.Printf("Error fetching untyped metric: %v", err)
	}

	log.Printf("Untyped metric values: %v", values)
	require.Empty(t, values, "Untyped metric was found when it should have been filtered out")
}

func verifyOtherMetricsPresence(t *testing.T) {
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

		if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(m.name, values, 0) {
			log.Printf("Values for metric %s are not as expected. Values: %v", m.name, values)
		}
		require.True(t, metric.IsAllValuesGreaterThanOrEqualToExpectedValue(m.name, values, 0),
			fmt.Sprintf("Values for metric %s are not as expected", m.name))
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
}
