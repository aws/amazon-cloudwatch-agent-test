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
	results := make([]status.TestResult, 0, len(metrics)+1)
	successCount := 0
	for _, m := range metrics {
		promql := fmt.Sprintf(`{__name__="%s"}`, m)
		resp, err := awsservice.QueryOtlpMetricsWithRetry(region, promql, 10, 30*time.Second)
		if err != nil {
			results = append(results, status.TestResult{Name: m, Status: status.FAILED, Reason: err})
			continue
		}
		if len(resp.Data.Result) == 0 {
			results = append(results, status.TestResult{Name: m, Status: status.FAILED, Reason: fmt.Errorf("no results for %s", m)})
			continue
		}
		results = append(results, status.TestResult{Name: m, Status: status.SUCCESSFUL})
		successCount++
	}
	if successCount != len(metrics) {
		results = append(results, status.TestResult{
			Name:   "MetricCountCheck",
			Status: status.FAILED,
			Reason: fmt.Errorf("expected %d metrics, but only %d were successfully validated", len(metrics), successCount),
		})
	}
	return status.TestGroupResult{Name: testName, TestResults: results}
}
