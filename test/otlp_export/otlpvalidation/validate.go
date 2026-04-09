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
	results := make([]status.TestResult, len(metrics))
	for i, m := range metrics {
		promql := fmt.Sprintf(`{__name__="%s"}`, m)
		resp, err := awsservice.QueryOtlpMetricsWithRetry(region, promql, 10, 30*time.Second)
		if err != nil {
			results[i] = status.TestResult{Name: m, Status: status.FAILED, Reason: err}
			continue
		}
		if len(resp.Data.Result) == 0 {
			results[i] = status.TestResult{Name: m, Status: status.FAILED, Reason: fmt.Errorf("no results for %s", m)}
			continue
		}
		results[i] = status.TestResult{Name: m, Status: status.SUCCESSFUL}
	}
	return status.TestGroupResult{Name: testName, TestResults: results}
}
