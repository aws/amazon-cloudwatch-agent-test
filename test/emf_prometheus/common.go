// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package emf_prometheus

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	namespacePrefix = "emf_prometheus_"
	logGroupPrefix  = "prometheus_test_"
)

func setupPrometheus(prometheusConfig, prometheusMetrics string, jobName string) error {
	var configContent string
	if jobName != "" {
		configContent = strings.Replace(prometheusConfig,
			"job_name: 'prometheus_test_job'",
			fmt.Sprintf("job_name: '%s'", jobName),
			1)
	} else {
		configContent = prometheusConfig
	}

	// Allow current user to write to /tmp directory
	if _, err := common.RunCommand("sudo chmod 777 /tmp"); err != nil {
		return fmt.Errorf("unable to chmod /tmp: %w", err)
	}
	fmt.Println(configContent)
	fmt.Println(prometheusMetrics)
	if err := os.WriteFile("/tmp/prometheus.yaml", []byte(configContent), os.ModePerm); err != nil {
		return fmt.Errorf("unable to write to /tmp/prometheus.yaml: %w", err)
	}
	if err := os.WriteFile("/tmp/metrics", []byte(prometheusMetrics), os.ModePerm); err != nil {
		return fmt.Errorf("unable to write to /tmp/metrics: %w", err)
	}
	commands := []string{
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}
	err := common.RunCommands(commands)
	if err != nil {
		return fmt.Errorf("failed to setup Prometheus: %v", err)
	}

	// Wait for server to start
	time.Sleep(2 * time.Second)
	return nil
}

func cleanup(logGroupName string) {
	commands := []string{
		"sudo pkill -f 'python3 -m http.server 8101'",
		"sudo rm -f /tmp/prometheus.yaml /tmp/metrics",
	}

	if err := common.RunCommands(commands); err != nil {
		log.Printf("failed to cleanup: %v", err)
	}

	awsservice.DeleteLogGroup(logGroupName)
}

func verifyRelabeledMetricsInCloudWatch(namespace string) status.TestResult {
	return verifyMetricsInCloudWatchWithDimensions(namespace, "Relabeled Metrics Presence", func(m struct{ name, promType string }) []types.Dimension {
		return []types.Dimension{{
			Name:  aws.String("my_replacement_test"),
			Value: aws.String(fmt.Sprintf("yes/%s", m.promType)),
		}}
	})
}

func verifyMetricsInCloudWatch(namespace string) status.TestResult {
	return verifyMetricsInCloudWatchWithDimensions(namespace, "Metrics Presence", func(m struct{ name, promType string }) []types.Dimension {
		return []types.Dimension{{
			Name:  aws.String("prom_type"),
			Value: aws.String(m.promType),
		}}
	})
}

func verifyMetricsInCloudWatchWithDimensions(namespace, testName string, dimensionFunc func(struct{ name, promType string }) []types.Dimension) status.TestResult {
	testResult := status.TestResult{
		Name:   testName,
		Status: status.FAILED,
	}

	metricsToCheck := []struct {
		name     string
		promType string
	}{
		{"prometheus_test_counter", "counter"},
		{"prometheus_test_gauge", "gauge"},
		{"prometheus_test_summary_sum", "summary"},
	}

	valueFetcher := metric.MetricValueFetcher{}
	maxRetries := 12
	retryInterval := 10 * time.Second

	for _, m := range metricsToCheck {
		log.Printf("Checking metric %s of type %s...", m.name, m.promType)
		dims := dimensionFunc(m)

		success := false
		for attempt := 1; attempt <= maxRetries; attempt++ {
			values, err := valueFetcher.Fetch(namespace, m.name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
			if err != nil {
				log.Printf("Attempt %d: Failed to fetch metric %s: %v", attempt, m.name, err)
			} else if len(values) == 0 {
				log.Printf("Attempt %d: No values found for metric %s", attempt, m.name)
			} else {
				log.Printf("Found %d values for metric %s: %v", len(values), m.name, values)
				success = true
				break
			}
			time.Sleep(retryInterval)
		}

		if !success {
			log.Printf("Metric %s not found after %d attempts", m.name, maxRetries)
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
