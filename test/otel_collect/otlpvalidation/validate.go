// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlpvalidation

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

const (
	defaultMaxRetries    = 3
	defaultRetryInterval = 30 * time.Second
)

func getRegion(region string) string {
	if region != "" {
		return region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-west-2"
}

func ValidateOtlpMetrics(testName string, region string, metrics []string) status.TestGroupResult {
	return ValidateOtlpMetricsWithLabels(testName, region, metrics, nil)
}

func ValidateOtlpMetricsWithLabels(testName string, region string, metrics []string, labels map[string]string) status.TestGroupResult {
	region = getRegion(region)

	client, err := otelmetrics.NewClient(context.Background(), otelmetrics.TestConfig{
		Region:         region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		SigningService: "monitoring",
	})
	if err != nil {
		return status.TestGroupResult{
			Name: testName,
			TestResults: []status.TestResult{{
				Name:   "ClientInit",
				Status: status.FAILED,
				Reason: fmt.Errorf("failed to create otelmetrics client: %w", err),
			}},
		}
	}

	validated := make(map[string]bool, len(metrics))

	for attempt := 0; attempt < defaultMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(defaultRetryInterval)
		}
		for _, m := range metrics {
			if validated[m] {
				continue
			}
			promql := fmt.Sprintf(`{__name__="%s"`, m)
			for k, v := range labels {
				promql += fmt.Sprintf(`, "%s"=~"%s"`, k, v)
			}
			promql += "}"
			results, err := client.Query(context.Background(), promql)
			if err != nil {
				log.Printf("[%s] attempt %d: error querying %s: %v", testName, attempt+1, m, err)
				continue
			}
			if len(results) == 0 {
				continue
			}
			validated[m] = true
		}
		if len(validated) == len(metrics) {
			break
		}
		log.Printf("[%s] attempt %d/%d: validated %d/%d metrics", testName, attempt+1, defaultMaxRetries, len(validated), len(metrics))
	}

	results := make([]status.TestResult, 0, len(metrics)+1)
	for _, m := range metrics {
		if validated[m] {
			results = append(results, status.TestResult{Name: m, Status: status.SUCCESSFUL})
		} else {
			results = append(results, status.TestResult{Name: m, Status: status.FAILED, Reason: fmt.Errorf("metric %s not found after %d retries", m, defaultMaxRetries)})
		}
	}
	successCount := len(validated)
	if successCount != len(metrics) {
		results = append(results, status.TestResult{
			Name:   "MetricCountCheck",
			Status: status.FAILED,
			Reason: fmt.Errorf("expected %d metrics, but only %d were successfully validated", len(metrics), successCount),
		})
	}
	return status.TestGroupResult{Name: testName, TestResults: results}
}
