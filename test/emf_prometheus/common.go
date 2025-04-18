// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package emf_prometheus

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
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

	commands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus.yaml\n%s\nEOF", configContent),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
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

func cleanup(logGroupName string) error {
	commands := []string{
		"sudo pkill -f 'python3 -m http.server 8101'",
		"sudo rm -f /tmp/prometheus.yaml /tmp/metrics",
	}

	if err := common.RunCommands(commands); err != nil {
		return fmt.Errorf("failed to cleanup: %v", err)
	}

	awsservice.DeleteLogGroup(logGroupName)
	return nil
}

func generateRandomSuffix() string {
	return strconv.Itoa(rand.Intn(100000))
}

func verifyMetricsInCloudWatch(namespace string) status.TestResult {
	testResult := status.TestResult{
		Name:   "Metrics Presence",
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
	for _, m := range metricsToCheck {
		log.Printf("Checking metric %s of type %s...", m.name, m.promType)
		dims := []types.Dimension{{
			Name:  aws.String("prom_type"),
			Value: aws.String(m.promType),
		}}

		values, err := valueFetcher.Fetch(namespace, m.name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
		if err != nil {
			log.Printf("Failed to fetch metric %s: %v", m.name, err)
			return testResult
		}

		if len(values) == 0 {
			log.Printf("No values found for metric %s", m.name)
			return testResult
		}
		log.Printf("Found %d values for metric %s: %v", len(values), m.name, values)
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
