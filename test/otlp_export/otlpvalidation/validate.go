// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otlpvalidation

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func ValidateOtlpMetrics(testName string, region string, metrics []string) status.TestGroupResult {
	const maxRetries = 10
	const retryInterval = 30 * time.Second

	// Track which metrics have been validated
	validated := make(map[string]bool, len(metrics))
	var lastFailures []string

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}
		lastFailures = nil
		for _, m := range metrics {
			if validated[m] {
				continue
			}
			promql := fmt.Sprintf(`{__name__="%s"}`, m)
			resp, err := awsservice.QueryOtlpMetrics(region, promql)
			if err != nil || len(resp.Data.Result) == 0 {
				lastFailures = append(lastFailures, m)
				continue
			}
			validated[m] = true
		}
		if len(lastFailures) == 0 {
			break
		}
	}

	results := make([]status.TestResult, 0, len(metrics)+1)
	for _, m := range metrics {
		if validated[m] {
			results = append(results, status.TestResult{Name: m, Status: status.SUCCESSFUL})
		} else {
			results = append(results, status.TestResult{Name: m, Status: status.FAILED, Reason: fmt.Errorf("metric %s not found after %d retries", m, maxRetries)})
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
